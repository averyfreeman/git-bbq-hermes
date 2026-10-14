package gitbbq

import (
	"fmt"
	"reflect"
	"strings"
)

// GitRunResult is the process result returned by a Git runner adapter.
type GitRunResult struct {
	ExitCode int
}

// GitRunner is the external-process seam for executing one already-planned command.
type GitRunner interface {
	Run(workspace string, command []string) (GitRunResult, error)
}

// GitExecution records the observable result of one approved Git plan.
type GitExecution struct {
	SchemaVersion    string     `json:"schema_version"`
	Action           GitAction  `json:"action"`
	Command          []string   `json:"command"`
	FollowUpCommands [][]string `json:"follow_up_commands,omitempty"`
	Executed         bool       `json:"executed"`
	ExitCode         int        `json:"exit_code"`
}

// ExecuteGitPlan runs one policy-approved plan through the supplied runner.
// Approval is required at this seam even when the project profile is autonomous.
func ExecuteGitPlan(config GitHabits, workspace string, plan GitPlan, approved bool, runner GitRunner) (GitExecution, error) {
	if strings.TrimSpace(workspace) == "" {
		return GitExecution{}, fmt.Errorf("Git execution requires a workspace")
	}
	if err := ValidateGitHabits(config); err != nil {
		return GitExecution{}, err
	}
	if !plan.Allowed || !config.Allows(plan.Action) {
		return GitExecution{}, fmt.Errorf("githabits policy denied %s", plan.Action)
	}
	if plan.NeedsInput {
		return GitExecution{}, fmt.Errorf("Git action %s requires input before it can be executed", plan.Action)
	}
	if !plan.RequiresApproval || !approved {
		return GitExecution{}, fmt.Errorf("Git action %s requires explicit approval", plan.Action)
	}
	if runner == nil {
		return GitExecution{}, fmt.Errorf("Git execution requires a runner")
	}
	if err := validateExecutableGitPlan(config, plan); err != nil {
		return GitExecution{}, err
	}
	expectedFollowUps := automaticPushFollowUps(config, plan.Action, plan.Command)
	if len(expectedFollowUps) > 0 && !config.Allows(GitActionPush) {
		return GitExecution{}, fmt.Errorf("Git action %s requires a push that the current policy or remote does not allow", plan.Action)
	}
	if !reflect.DeepEqual(plan.FollowUpCommands, expectedFollowUps) {
		return GitExecution{}, fmt.Errorf("Git plan follow-up commands do not match the current push cadence")
	}

	commands := append([][]string{plan.Command}, plan.FollowUpCommands...)
	if containsGitPush(commands) {
		if err := validateConfiguredPushDestination(workspace, config); err != nil {
			return GitExecution{}, err
		}
	}
	for index, command := range commands {
		result, err := runner.Run(workspace, append([]string(nil), command...))
		if err != nil {
			if index > 0 {
				return GitExecution{}, fmt.Errorf("Git action %s succeeded but its required follow-up push failed: %w", plan.Action, err)
			}
			return GitExecution{}, fmt.Errorf("Git action %s failed: %w", plan.Action, err)
		}
		if result.ExitCode != 0 {
			if index > 0 {
				return GitExecution{}, fmt.Errorf("Git action %s succeeded but its required follow-up push exited with code %d", plan.Action, result.ExitCode)
			}
			return GitExecution{}, fmt.Errorf("Git action %s exited with code %d", plan.Action, result.ExitCode)
		}
	}
	return GitExecution{
		SchemaVersion:    SchemaVersion,
		Action:           plan.Action,
		Command:          append([]string(nil), plan.Command...),
		FollowUpCommands: cloneCommands(plan.FollowUpCommands),
		Executed:         true,
		ExitCode:         0,
	}, nil
}

func cloneCommands(commands [][]string) [][]string {
	cloned := make([][]string, len(commands))
	for index, command := range commands {
		cloned[index] = append([]string(nil), command...)
	}
	return cloned
}

func validateExecutableGitPlan(config GitHabits, plan GitPlan) error {
	if plan.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported Git plan schema_version %q", plan.SchemaVersion)
	}
	if len(plan.Command) == 0 || (plan.Command[0] != "git" && plan.Command[0] != "gh") {
		return fmt.Errorf("Git plan has no safe command")
	}
	for _, argument := range plan.Command[1:] {
		if strings.ContainsAny(argument, "\x00\r\n") || argument == "--force" || argument == "--force-with-lease" || argument == "-f" {
			return fmt.Errorf("Git plan contains a forbidden argument")
		}
	}

	switch plan.Action {
	case GitActionInit:
		if len(plan.Command) != 2 || plan.Command[1] != "init" {
			return fmt.Errorf("Git init plan has an invalid command")
		}
	case GitActionBranch:
		if len(plan.Command) != 4 || plan.Command[1] != "switch" || plan.Command[2] != "-c" || !safeGitRef(plan.Command[3]) {
			return fmt.Errorf("Git branch plan has an invalid command")
		}
	case GitActionRemote:
		if plan.Command[0] == "gh" {
			return validateRemoteProvisionCommand(plan.Command)
		}
		if len(plan.Command) != 5 || plan.Command[1] != "remote" || plan.Command[2] != "add" || plan.Command[3] != config.Remote.Alias || plan.Command[4] != config.Remote.URL || !safeGitRef(plan.Command[3]) || !safeGitRemoteURL(plan.Command[4]) {
			return fmt.Errorf("Git remote plan has an invalid command")
		}
	case GitActionStage:
		if len(plan.Command) < 4 || plan.Command[1] != "add" || plan.Command[2] != "--" {
			return fmt.Errorf("Git stage plan has an invalid command")
		}
		for _, path := range plan.Command[3:] {
			if !safeGitPath(path) {
				return fmt.Errorf("Git stage plan contains an unsafe path")
			}
		}
	case GitActionCommit:
		if len(plan.Command) != 4 || plan.Command[1] != "commit" || plan.Command[2] != "-m" || strings.TrimSpace(plan.Command[3]) == "" {
			return fmt.Errorf("Git commit plan has an invalid command")
		}
	case GitActionTag:
		if len(plan.Command) != 6 || plan.Command[1] != "tag" || plan.Command[2] != "-a" || plan.Command[4] != "-m" || plan.Command[3] != plan.Command[5] || !safeGitRef(plan.Command[3]) {
			return fmt.Errorf("Git tag plan has an invalid command")
		}
	case GitActionPush:
		if len(plan.Command) != 4 || plan.Command[1] != "push" || plan.Command[2] != config.Remote.Alias || !safeGitRef(plan.Command[2]) {
			return fmt.Errorf("Git push plan has an invalid command")
		}
		if strings.HasPrefix(plan.Command[3], "refs/tags/") {
			tag := strings.TrimPrefix(plan.Command[3], "refs/tags/")
			if !safeGitRef(tag) || !safeSemanticVersionTag(tag) {
				return fmt.Errorf("Git push plan has an invalid tag ref")
			}
		} else if !safeGitRef(plan.Command[3]) {
			return fmt.Errorf("Git push plan has an invalid branch ref")
		}
	default:
		return fmt.Errorf("Git action %q is not executable yet", plan.Action)
	}
	return nil
}

func containsGitPush(commands [][]string) bool {
	for _, command := range commands {
		if len(command) >= 2 && command[0] == "git" && command[1] == "push" {
			return true
		}
	}
	return false
}

func validateConfiguredPushDestination(workspace string, config GitHabits) error {
	if config.Remote.Status != "configured" || !safeGitRef(config.Remote.Alias) || !safeGitRemoteURL(config.Remote.URL) {
		return fmt.Errorf("push requires the configured GitHabits remote")
	}
	for _, arguments := range [][]string{
		{"remote", "get-url", config.Remote.Alias},
		{"remote", "get-url", "--push", config.Remote.Alias},
	} {
		remoteURL, err := readGitOutput(workspace, arguments...)
		if err != nil || strings.TrimSpace(remoteURL) != strings.TrimSpace(config.Remote.URL) {
			return fmt.Errorf("selected checkout push destination does not match configured GitHabits remote")
		}
	}
	return nil
}
