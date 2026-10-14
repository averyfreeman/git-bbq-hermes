package gitbbq

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var conventionalCommitPattern = regexp.MustCompile(`^[^[:space:]():!]+(\([^)]*\))?!?: .+`)
var semanticVersionTagPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
var scpGitRemotePattern = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+@)?[A-Za-z0-9-]+(?:[.][A-Za-z0-9-]+)+:[A-Za-z0-9._~/-][^?#[:space:]]*$`)

// GitPlanRequest contains the values needed to render one Git action preview.
// It deliberately carries data, not permission; approval remains a host concern.
type GitPlanRequest struct {
	Branch       string
	Message      string
	Tag          string
	Paths        []string
	ExistingTags []string
}

// GitPlan is the read-only result of applying githabits policy to one action,
// including any push commands required by the configured cadence.
type GitPlan struct {
	SchemaVersion    string     `json:"schema_version"`
	Action           GitAction  `json:"action"`
	Allowed          bool       `json:"allowed"`
	RequiresApproval bool       `json:"requires_approval"`
	NeedsInput       bool       `json:"needs_input,omitempty"`
	RequiredInputs   []string   `json:"required_inputs,omitempty"`
	Reason           string     `json:"reason"`
	Command          []string   `json:"command"`
	FollowUpCommands [][]string `json:"follow_up_commands,omitempty"`
}

// PlanGitAction renders one safe Git command without executing it.
func PlanGitAction(config GitHabits, action GitAction, request GitPlanRequest) (GitPlan, error) {
	if err := ValidateGitHabits(config); err != nil {
		return GitPlan{}, err
	}

	plan := GitPlan{
		SchemaVersion:    SchemaVersion,
		Action:           action,
		Allowed:          config.Allows(action),
		RequiresApproval: true,
	}
	if plan.Allowed {
		plan.Reason = "githabits policy allows this action; host approval is still required"
	} else {
		plan.Reason = fmt.Sprintf("githabits policy does not authorize %s", action)
	}

	switch action {
	case GitActionInit:
		plan.Command = []string{"git", "init"}
	case GitActionBranch:
		branch := strings.TrimSpace(request.Branch)
		if branch == "" {
			branch = config.Branch
		}
		if !safeGitRef(branch) {
			return GitPlan{}, fmt.Errorf("branch is not a safe Git ref: %q", branch)
		}
		plan.Command = []string{"git", "switch", "-c", branch}
	case GitActionRemote:
		if config.Remote.Status != "pending" {
			return GitPlan{}, fmt.Errorf("remote planning requires a pending remote")
		}
		remoteURL := strings.TrimSpace(config.Remote.URL)
		if !safeGitRemoteURL(remoteURL) {
			return GitPlan{}, fmt.Errorf("remote URL is empty or unsafe")
		}
		plan.Command = []string{"git", "remote", "add", config.Remote.Alias, remoteURL}
	case GitActionStage:
		if len(request.Paths) == 0 {
			return GitPlan{}, fmt.Errorf("stage planning requires at least one path")
		}
		command := []string{"git", "add", "--"}
		for _, path := range request.Paths {
			if !safeGitPath(path) {
				return GitPlan{}, fmt.Errorf("stage path is not a safe repository-relative path: %q", path)
			}
			command = append(command, path)
		}
		plan.Command = command
	case GitActionTag:
		tag := strings.TrimSpace(request.Tag)
		if tag == "" {
			return GitPlan{}, fmt.Errorf("tag planning requires a tag name")
		}
		if !safeGitRef(tag) {
			return GitPlan{}, fmt.Errorf("tag is not a safe Git ref: %q", tag)
		}
		if config.Versioning == "semver" && !safeSemanticVersionTag(tag) {
			return GitPlan{}, fmt.Errorf("tag does not follow the configured semver convention: %q", tag)
		}
		if err := ValidateTagProgression(request.ExistingTags, tag); err != nil {
			return GitPlan{}, err
		}
		plan.Command = []string{"git", "tag", "-a", tag, "-m", tag}
	case GitActionPush:
		if !plan.Allowed {
			return plan, nil
		}
		if strings.TrimSpace(request.Tag) != "" {
			if strings.TrimSpace(request.Branch) != "" {
				return GitPlan{}, fmt.Errorf("push planning accepts either a branch or a tag, not both")
			}
			tag := strings.TrimSpace(request.Tag)
			if !safeGitRef(tag) {
				return GitPlan{}, fmt.Errorf("push tag is not a safe Git ref: %q", tag)
			}
			if config.Versioning == "semver" && !safeSemanticVersionTag(tag) {
				return GitPlan{}, fmt.Errorf("push tag does not follow the configured semver convention: %q", tag)
			}
			if err := ValidateTagProgression(request.ExistingTags, tag); err != nil {
				return GitPlan{}, err
			}
			plan.Command = []string{"git", "push", config.Remote.Alias, "refs/tags/" + tag}
			break
		}
		branch := strings.TrimSpace(request.Branch)
		if branch == "" {
			if config.Push.Target == "current" {
				branch = "HEAD"
			} else {
				branch = config.Branch
			}
		}
		if !safeGitRef(branch) {
			return GitPlan{}, fmt.Errorf("push branch is not a safe Git ref: %q", branch)
		}
		plan.Command = []string{"git", "push", config.Remote.Alias, branch}
	case GitActionCommit:
		message := strings.TrimSpace(request.Message)
		if message == "" {
			return GitPlan{}, fmt.Errorf("commit planning requires a message")
		}
		if config.CommitStyle == "conventional-commits" && !conventionalCommitPattern.MatchString(message) {
			return GitPlan{}, fmt.Errorf("commit message does not follow conventional-commits style")
		}
		plan.Command = []string{"git", "commit", "-m", message}
	default:
		return GitPlan{}, fmt.Errorf("Git action %q is not supported by the planner yet", action)
	}

	followUps := automaticPushFollowUps(config, action, plan.Command)
	if len(followUps) > 0 && plan.Allowed {
		if !config.Actions.Push {
			plan.Allowed = false
			plan.Reason = fmt.Sprintf("%s is denied because its required push is not authorized", action)
		} else if config.Remote.Status != "configured" {
			plan.Allowed = false
			plan.NeedsInput = true
			plan.RequiredInputs = []string{"remote"}
			plan.Reason = fmt.Sprintf("%s push cadence requires a configured remote", action)
		} else if !config.Allows(GitActionPush) {
			plan.Allowed = false
			plan.Reason = fmt.Sprintf("%s is denied because its required push is not authorized", action)
		} else {
			plan.FollowUpCommands = followUps
		}
	}
	if !plan.Allowed {
		plan.Command = nil
		plan.FollowUpCommands = nil
	}
	return plan, nil
}

func automaticPushFollowUps(config GitHabits, action GitAction, command []string) [][]string {
	switch action {
	case GitActionCommit:
		if config.Version != 2 || !config.Push.AfterCommit {
			return nil
		}
		branch := "HEAD"
		if config.Push.Target != "current" {
			branch = config.Branch
		}
		return [][]string{{"git", "push", config.Remote.Alias, branch}}
	case GitActionTag:
		if config.Version != 2 || !config.Push.AfterTag || len(command) < 4 {
			return nil
		}
		return [][]string{{"git", "push", config.Remote.Alias, "refs/tags/" + command[3]}}
	default:
		return nil
	}
}

func safeGitPath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") {
		return false
	}
	if len(value) >= 2 && value[1] == ':' {
		return false
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return false
		}
	}
	return !strings.Contains(value, "..\\") && !strings.Contains(value, "../")
}

func safeGitRemoteURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, " 	\x00\r\n") {
		return false
	}
	if scpGitRemotePattern.MatchString(value) {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.User == nil && !strings.ContainsAny(value, "?#")
}

func safeSemanticVersionTag(value string) bool {
	match := semanticVersionTagPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) == 0 {
		return false
	}
	for _, identifier := range strings.Split(strings.TrimPrefix(match[4], "-"), ".") {
		if len(identifier) < 2 || identifier[0] != '0' {
			continue
		}
		numeric := true
		for index := 0; index < len(identifier); index++ {
			if identifier[index] < '0' || identifier[index] > '9' {
				numeric = false
				break
			}
		}
		if numeric {
			return false
		}
	}
	return true
}
