package gitbbq

import (
	"reflect"
	"testing"
)

func TestPlanRemoteProvisionRequiresExplicitVisibility(t *testing.T) {
	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	config.Remote.Owner = "averyfreeman"
	config.Remote.Name = "example-project"

	plan, err := PlanRemoteProvision(config)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.NeedsInput || !reflect.DeepEqual(plan.RequiredInputs, []string{"remote.visibility"}) || len(plan.Command) != 0 {
		t.Fatalf("unspecified remote visibility was not returned as a resolvable requirement: %#v", plan)
	}
}

func TestPlanRemoteProvisionBuildsApprovalGatedGitHubCommand(t *testing.T) {
	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	config.Remote.Owner = "averyfreeman"
	config.Remote.Name = "example-project"
	config.Remote.Visibility = "private"

	plan, err := PlanRemoteProvision(config)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Allowed || !plan.RequiresApproval {
		t.Fatalf("remote provision plan = %#v", plan)
	}
	if want := []string{"gh", "repo", "create", "averyfreeman/example-project", "--private", "--source", ".", "--remote", "origin"}; !reflect.DeepEqual(plan.Command, want) {
		t.Fatalf("command = %#v, want %#v", plan.Command, want)
	}
}

func TestExecuteRemoteProvisionRunsApprovedGhCommand(t *testing.T) {
	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	config.Remote.Owner = "averyfreeman"
	config.Remote.Name = "example-project"
	config.Remote.Visibility = "private"
	plan, err := PlanRemoteProvision(config)
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingGitRunner{}

	execution, err := ExecuteGitPlan(config, "/workspace", plan, true, runner)
	if err != nil {
		t.Fatal(err)
	}
	if !execution.Executed || !reflect.DeepEqual(runner.command, plan.Command) {
		t.Fatalf("execution = %#v, command = %#v", execution, runner.command)
	}
}
