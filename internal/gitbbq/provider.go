package gitbbq

import (
	"fmt"
	"regexp"
	"strings"
)

var remoteComponentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// PlanRemoteProvision renders an approval-gated GitHub repository creation command.
// It never includes --push; publication remains a separate Git action.
func PlanRemoteProvision(config GitHabits) (GitPlan, error) {
	if err := ValidateGitHabits(config); err != nil {
		return GitPlan{}, err
	}
	if config.Remote.Provider != "github" {
		return GitPlan{}, fmt.Errorf("remote provisioning does not support provider %q", config.Remote.Provider)
	}
	if config.Remote.Status != "pending" {
		return GitPlan{}, fmt.Errorf("remote provisioning requires a pending remote")
	}
	if strings.TrimSpace(config.Remote.URL) != "" {
		return GitPlan{}, fmt.Errorf("remote provisioning cannot replace an existing remote URL")
	}
	if config.Remote.Visibility == "" {
		return GitPlan{}, fmt.Errorf("remote visibility is unspecified; ask the user before creating the remote")
	}
	if !safeRemoteComponent(config.Remote.Owner) || !safeRemoteComponent(config.Remote.Name) {
		return GitPlan{}, fmt.Errorf("remote provisioning requires safe owner and repository name")
	}

	plan := GitPlan{
		SchemaVersion:    SchemaVersion,
		Action:           GitActionRemote,
		Allowed:          config.Allows(GitActionRemote),
		RequiresApproval: true,
		Reason:           "githabits policy allows remote provisioning; host approval is still required",
	}
	if !plan.Allowed {
		plan.Reason = "githabits policy does not authorize remote provisioning"
		return plan, nil
	}
	owner := strings.TrimSpace(config.Remote.Owner)
	name := strings.TrimSpace(config.Remote.Name)
	plan.Command = []string{
		"gh", "repo", "create", owner + "/" + name,
		"--" + config.Remote.Visibility,
		"--source", ".",
		"--remote", strings.TrimSpace(config.Remote.Alias),
	}
	return plan, nil
}

func safeRemoteComponent(value string) bool {
	return remoteComponentPattern.MatchString(strings.TrimSpace(value))
}

func validateRemoteProvisionCommand(command []string) error {
	if len(command) != 9 || command[0] != "gh" || command[1] != "repo" || command[2] != "create" {
		return fmt.Errorf("GitHub remote provision plan has an invalid command")
	}
	parts := strings.Split(command[3], "/")
	if len(parts) != 2 || !safeRemoteComponent(parts[0]) || !safeRemoteComponent(parts[1]) {
		return fmt.Errorf("GitHub remote provision plan has an unsafe repository name")
	}
	if command[4] != "--private" && command[4] != "--public" && command[4] != "--internal" {
		return fmt.Errorf("GitHub remote provision plan has an invalid visibility")
	}
	if command[5] != "--source" || command[6] != "." || command[7] != "--remote" || !safeGitRef(command[8]) {
		return fmt.Errorf("GitHub remote provision plan has an invalid source or alias")
	}
	return nil
}
