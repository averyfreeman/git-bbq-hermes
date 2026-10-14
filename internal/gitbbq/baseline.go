package gitbbq

import (
	"fmt"
	"os/exec"
	"reflect"
	"sort"
	"strings"
)

// GitBaselinePlan is a read-only, approval-gated initial release plan. It only
// removes local tags proven to come from upstream and absent from origin.
type GitBaselinePlan struct {
	SchemaVersion     string     `json:"schema_version"`
	Allowed           bool       `json:"allowed"`
	RequiresApproval  bool       `json:"requires_approval"`
	Tag               string     `json:"tag"`
	LocalTagsToRemove []string   `json:"local_tags_to_remove"`
	RemoteTagsKept    []string   `json:"remote_tags_kept"`
	Commands          [][]string `json:"commands"`
	Reason            string     `json:"reason"`
}

// GitBaselineExecution records the completed initial baseline workflow.
type GitBaselineExecution struct {
	SchemaVersion string     `json:"schema_version"`
	Tag           string     `json:"tag"`
	Commands      [][]string `json:"commands"`
	Executed      bool       `json:"executed"`
	ExitCode      int        `json:"exit_code"`
}

// PlanInitialBaseline identifies imported local tags and plans the initial tag
// without deleting or rewriting any remote ref.
func PlanInitialBaseline(config GitHabits, workspace string) (GitBaselinePlan, error) {
	if err := ValidateGitHabits(config); err != nil {
		return GitBaselinePlan{}, err
	}
	if strings.TrimSpace(workspace) == "" {
		return GitBaselinePlan{}, fmt.Errorf("initial baseline planning requires a workspace")
	}
	if config.Release.InitialTagScope != "new-repository-only" {
		return GitBaselinePlan{}, fmt.Errorf("initial baseline is disabled by GitHabits policy")
	}
	if config.Versioning != "semver" || !safeSemanticVersionTag(config.InitialTag) {
		return GitBaselinePlan{}, fmt.Errorf("initial baseline requires a configured SemVer tag")
	}
	if !config.Allows(GitActionTag) || !config.Allows(GitActionPush) {
		return GitBaselinePlan{}, fmt.Errorf("initial baseline requires tag and push actions to be allowed")
	}
	if config.Remote.Status != "configured" || !safeGitRef(config.Remote.Alias) || !safeGitRemoteURL(config.Remote.URL) {
		return GitBaselinePlan{}, fmt.Errorf("initial baseline requires the configured remote to be available")
	}

	configuredRemote, err := readGitOutput(workspace, "remote", "get-url", config.Remote.Alias)
	if err != nil || strings.TrimSpace(configuredRemote) != strings.TrimSpace(config.Remote.URL) {
		return GitBaselinePlan{}, fmt.Errorf("selected checkout remote does not match configured GitHabits remote")
	}
	localOutput, err := readGitOutput(workspace, "for-each-ref", "--format=%(refname:short)%00%(objectname)", "refs/tags")
	if err != nil {
		return GitBaselinePlan{}, fmt.Errorf("read local tags for initial baseline: %w", err)
	}
	originOutput, err := readGitOutput(workspace, "ls-remote", "--tags", "--refs", config.Remote.Alias)
	if err != nil {
		return GitBaselinePlan{}, fmt.Errorf("read configured remote tags for initial baseline: %w", err)
	}
	localTags := parseLocalTagObjectIDs(localOutput)
	originTags := parseRemoteTagObjectIDs(originOutput)
	upstreamTags := map[string]string{}
	if _, err := readGitOutput(workspace, "remote", "get-url", "upstream"); err == nil {
		upstreamOutput, err := readGitOutput(workspace, "ls-remote", "--tags", "--refs", "upstream")
		if err != nil {
			return GitBaselinePlan{}, fmt.Errorf("read upstream tags for initial baseline: %w", err)
		}
		upstreamTags = parseRemoteTagObjectIDs(upstreamOutput)
	}

	plan := GitBaselinePlan{
		SchemaVersion:    SchemaVersion,
		Allowed:          true,
		RequiresApproval: true,
		Tag:              config.InitialTag,
		Reason:           "remove only upstream-imported local tags absent from origin, preserve published tags, then create and push the configured initial baseline",
	}
	for tag := range originTags {
		plan.RemoteTagsKept = append(plan.RemoteTagsKept, tag)
	}
	sort.Strings(plan.RemoteTagsKept)
	for tag, localObjectID := range localTags {
		if tag == config.InitialTag {
			return GitBaselinePlan{}, fmt.Errorf("initial baseline tag %q already exists locally", tag)
		}
		upstreamObjectID, importedFromUpstream := upstreamTags[tag]
		_, publishedOnOrigin := originTags[tag]
		if importedFromUpstream && localObjectID == upstreamObjectID && !publishedOnOrigin {
			plan.LocalTagsToRemove = append(plan.LocalTagsToRemove, tag)
		}
	}
	sort.Strings(plan.LocalTagsToRemove)
	if _, exists := originTags[config.InitialTag]; exists {
		return GitBaselinePlan{}, fmt.Errorf("initial baseline tag %q already exists on the configured remote", config.InitialTag)
	}

	if len(plan.LocalTagsToRemove) > 0 {
		plan.Commands = append(plan.Commands, append([]string{"git", "tag", "-d"}, plan.LocalTagsToRemove...))
	}
	plan.Commands = append(plan.Commands,
		[]string{"git", "tag", "-a", config.InitialTag, "-m", config.InitialTag},
		[]string{"git", "push", config.Remote.Alias, "refs/tags/" + config.InitialTag},
	)
	return plan, nil
}

// ExecuteInitialBaseline rechecks the read-only plan and requires one explicit
// approval before running its local cleanup, tag creation, and tag push steps.
func ExecuteInitialBaseline(config GitHabits, workspace string, plan GitBaselinePlan, approved bool, runner GitRunner) (GitBaselineExecution, error) {
	if !approved {
		return GitBaselineExecution{}, fmt.Errorf("initial baseline requires explicit approval")
	}
	if runner == nil {
		return GitBaselineExecution{}, fmt.Errorf("initial baseline requires a Git runner")
	}
	current, err := PlanInitialBaseline(config, workspace)
	if err != nil {
		return GitBaselineExecution{}, err
	}
	if !reflect.DeepEqual(plan, current) {
		return GitBaselineExecution{}, fmt.Errorf("initial baseline plan is stale or has been modified")
	}
	for _, command := range current.Commands {
		if err := validateBaselineCommand(command, config, current); err != nil {
			return GitBaselineExecution{}, err
		}
	}
	for _, command := range current.Commands {
		result, err := runner.Run(workspace, append([]string(nil), command...))
		if err != nil {
			return GitBaselineExecution{}, fmt.Errorf("initial baseline command failed: %w", err)
		}
		if result.ExitCode != 0 {
			return GitBaselineExecution{}, fmt.Errorf("initial baseline command exited with code %d", result.ExitCode)
		}
	}
	return GitBaselineExecution{
		SchemaVersion: SchemaVersion,
		Tag:           current.Tag,
		Commands:      current.Commands,
		Executed:      true,
		ExitCode:      0,
	}, nil
}

func validateBaselineCommand(command []string, config GitHabits, plan GitBaselinePlan) error {
	if len(command) == 0 || command[0] != "git" {
		return fmt.Errorf("initial baseline contains an invalid command")
	}
	for _, argument := range command[1:] {
		if strings.ContainsAny(argument, "\x00\r\n") || argument == "--force" || argument == "-f" {
			return fmt.Errorf("initial baseline contains a forbidden argument")
		}
	}
	if len(command) == 3+len(plan.LocalTagsToRemove) && command[1] == "tag" && command[2] == "-d" {
		if len(command) < 4 || !reflect.DeepEqual(command[3:], plan.LocalTagsToRemove) {
			return fmt.Errorf("initial baseline local tag cleanup does not match the observed plan")
		}
		return nil
	}
	if reflect.DeepEqual(command, []string{"git", "tag", "-a", plan.Tag, "-m", plan.Tag}) {
		return nil
	}
	if reflect.DeepEqual(command, []string{"git", "push", config.Remote.Alias, "refs/tags/" + plan.Tag}) {
		return nil
	}
	return fmt.Errorf("initial baseline contains an unrecognized command")
}

func readGitOutput(workspace string, arguments ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", workspace}, arguments...)...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func parseLocalTagObjectIDs(output string) map[string]string {
	tags := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "\x00", 2)
		if len(parts) == 2 && safeGitRef(parts[0]) && parts[1] != "" {
			tags[parts[0]] = parts[1]
		}
	}
	return tags
}

func parseRemoteTagObjectIDs(output string) map[string]string {
	tags := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "	", 2)
		if len(parts) != 2 || !strings.HasPrefix(parts[1], "refs/tags/") {
			continue
		}
		ref := strings.TrimPrefix(parts[1], "refs/tags/")
		if strings.HasSuffix(ref, "^{}") || parts[0] == "" {
			continue
		}
		if safeGitRef(ref) {
			tags[ref] = parts[0]
		}
	}
	return tags
}
