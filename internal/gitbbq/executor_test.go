package gitbbq

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type recordingGitRunner struct {
	calls     int
	workspace string
	command   []string
	commands  [][]string
	failAt    int
}

func (runner *recordingGitRunner) Run(workspace string, command []string) (GitRunResult, error) {
	runner.calls++
	runner.workspace = workspace
	runner.command = append([]string(nil), command...)
	runner.commands = append(runner.commands, append([]string(nil), command...))
	if runner.calls == runner.failAt {
		return GitRunResult{ExitCode: 1}, nil
	}
	return GitRunResult{ExitCode: 0}, nil
}

func setupExecutorTestRemote(t *testing.T, alias, remoteURL string) string {
	t.Helper()
	workspace := t.TempDir()
	for _, arguments := range [][]string{{"init", "-q"}, {"remote", "add", alias, remoteURL}} {
		if output, err := exec.Command("git", append([]string{"-C", workspace}, arguments...)...).CombinedOutput(); err != nil {
			t.Fatalf("set up Git execution remote: %v\n%s", err, output)
		}
	}
	return workspace
}

func TestExecuteGitPlanRequiresApprovalBeforeRunner(t *testing.T) {
	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanGitAction(config, GitActionCommit, GitPlanRequest{Message: "feat: add executor"})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingGitRunner{}

	if _, err := ExecuteGitPlan(config, "/workspace", plan, false, runner); err == nil {
		t.Fatal("unapproved Git plan was executed")
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestExecuteGitPlanRunsApprovedCommandThroughRunner(t *testing.T) {
	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	config.Remote.Status = "configured"
	config.Remote.URL = "https://github.com/example/project.git"
	workspace := setupExecutorTestRemote(t, config.Remote.Alias, config.Remote.URL)
	plan, err := PlanGitAction(config, GitActionCommit, GitPlanRequest{Message: "feat: add executor"})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingGitRunner{}

	execution, err := ExecuteGitPlan(config, workspace, plan, true, runner)
	if err != nil {
		t.Fatal(err)
	}
	if !execution.Executed || execution.ExitCode != 0 {
		t.Fatalf("execution = %#v", execution)
	}
	if runner.workspace != workspace {
		t.Fatalf("workspace = %q, want %q", runner.workspace, workspace)
	}
	if runner.calls != 2 {
		t.Fatalf("runner calls = %d, want commit followed by configured push", runner.calls)
	}
	if want := []string{"git", "commit", "-m", "feat: add executor"}; !reflect.DeepEqual(runner.commands[0], want) {
		t.Fatalf("commit command = %#v, want %#v", runner.commands[0], want)
	}
	if want := []string{"git", "push", "origin", "HEAD"}; !reflect.DeepEqual(runner.commands[1], want) {
		t.Fatalf("follow-up push command = %#v, want %#v", runner.commands[1], want)
	}
}

func TestExecuteGitPlanReportsRequiredPushFailureAfterCommit(t *testing.T) {
	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	config.Remote.Status = "configured"
	config.Remote.URL = "https://github.com/example/project.git"
	workspace := setupExecutorTestRemote(t, config.Remote.Alias, config.Remote.URL)
	plan, err := PlanGitAction(config, GitActionCommit, GitPlanRequest{Message: "feat: add executor"})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingGitRunner{failAt: 2}

	if _, err := ExecuteGitPlan(config, workspace, plan, true, runner); err == nil || !strings.Contains(err.Error(), "succeeded but its required follow-up push exited") {
		t.Fatalf("follow-up push failure = %v", err)
	}
	if runner.calls != 2 {
		t.Fatalf("runner calls = %d, want the commit and its failing push", runner.calls)
	}
}

func TestExecuteGitPlanRejectsForcePushFromTamperedPlan(t *testing.T) {
	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	config.Remote.Status = "configured"
	config.Remote.URL = "https://github.com/example/project.git"
	plan := GitPlan{
		SchemaVersion:    SchemaVersion,
		Action:           GitActionPush,
		Allowed:          true,
		RequiresApproval: true,
		Command:          []string{"git", "push", "--force", "origin", "main"},
	}
	runner := &recordingGitRunner{}

	if _, err := ExecuteGitPlan(config, "/workspace", plan, true, runner); err == nil {
		t.Fatal("force push was executed")
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestExecuteGitPlanRechecksCurrentPolicy(t *testing.T) {
	plannedConfig, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanGitAction(plannedConfig, GitActionCommit, GitPlanRequest{Message: "feat: add executor"})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingGitRunner{}

	if _, err := ExecuteGitPlan(DefaultGitHabits(), "/workspace", plan, true, runner); err == nil {
		t.Fatal("execution ignored the current githabits policy")
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestOSGitRunnerRunsGitInWorkspace(t *testing.T) {
	workspace := t.TempDir()
	runner := OSGitRunner{Timeout: time.Second}

	result, err := runner.Run(workspace, []string{"git", "init", "-q"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".git")); err != nil {
		t.Fatalf("git repository was not initialized: %v", err)
	}
}

func TestExecuteGitPlanInitializesRepository(t *testing.T) {
	workspace := t.TempDir()
	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanGitAction(config, GitActionInit, GitPlanRequest{})
	if err != nil {
		t.Fatal(err)
	}

	execution, err := ExecuteGitPlan(config, workspace, plan, true, OSGitRunner{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if !execution.Executed {
		t.Fatal("init plan did not execute")
	}
	if _, err := os.Stat(filepath.Join(workspace, ".git")); err != nil {
		t.Fatalf("git repository was not initialized: %v", err)
	}
}

func TestExecuteGitPlanCreatesBranch(t *testing.T) {
	workspace := t.TempDir()
	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	initPlan, err := PlanGitAction(config, GitActionInit, GitPlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, initPlan, true, OSGitRunner{Timeout: time.Second}); err != nil {
		t.Fatal(err)
	}
	branchPlan, err := PlanGitAction(config, GitActionBranch, GitPlanRequest{Branch: "feature/git-bbq"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, branchPlan, true, OSGitRunner{Timeout: time.Second}); err != nil {
		t.Fatal(err)
	}

	branch, err := exec.Command("git", "-C", workspace, "branch", "--show-current").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(branch); got != "feature/git-bbq\n" {
		t.Fatalf("current branch = %q", got)
	}
}

func TestExecuteGitPlanAddsPendingRemote(t *testing.T) {
	workspace := t.TempDir()
	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	config.Remote.URL = "https://github.com/example/project.git"
	runner := OSGitRunner{Timeout: time.Second}

	initPlan, err := PlanGitAction(config, GitActionInit, GitPlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, initPlan, true, runner); err != nil {
		t.Fatal(err)
	}
	remotePlan, err := PlanGitAction(config, GitActionRemote, GitPlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, remotePlan, true, runner); err != nil {
		t.Fatal(err)
	}

	remote, err := exec.Command("git", "-C", workspace, "remote", "get-url", "origin").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(remote); got != "https://github.com/example/project.git\n" {
		t.Fatalf("remote URL = %q", got)
	}
}

func TestExecuteGitPlanPushesCommitToConfiguredRemote(t *testing.T) {
	workspace := t.TempDir()
	bareRemote := filepath.Join(t.TempDir(), "remote.git")
	if err := exec.Command("git", "init", "--bare", "-q", bareRemote).Run(); err != nil {
		t.Fatal(err)
	}

	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	config.Remote.Status = "configured"
	config.Remote.URL = bareRemote
	runner := OSGitRunner{Timeout: time.Second}

	initPlan, err := PlanGitAction(config, GitActionInit, GitPlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, initPlan, true, runner); err != nil {
		t.Fatal(err)
	}
	branchPlan, err := PlanGitAction(config, GitActionBranch, GitPlanRequest{Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, branchPlan, true, runner); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(workspace, []string{"git", "config", "user.email", "git-bbq@example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(workspace, []string{"git", "config", "user.name", "Git BBQ Test"}); err != nil {
		t.Fatal(err)
	}

	pendingConfig := config
	pendingConfig.Remote.Status = "pending"
	remotePlan, err := PlanGitAction(pendingConfig, GitActionRemote, GitPlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(pendingConfig, workspace, remotePlan, true, runner); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("push tracer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stagePlan, err := PlanGitAction(config, GitActionStage, GitPlanRequest{Paths: []string{"README.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, stagePlan, true, runner); err != nil {
		t.Fatal(err)
	}
	commitPlan, err := PlanGitAction(config, GitActionCommit, GitPlanRequest{Message: "feat: add push tracer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, commitPlan, true, runner); err != nil {
		t.Fatal(err)
	}
	tagPlan, err := PlanGitAction(config, GitActionTag, GitPlanRequest{Tag: "v0.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, tagPlan, true, runner); err != nil {
		t.Fatal(err)
	}

	if _, err := exec.Command("git", "--git-dir", bareRemote, "show-ref", "--verify", "refs/heads/main").Output(); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.Command("git", "--git-dir", bareRemote, "show-ref", "--verify", "refs/tags/v0.1.0").Output(); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteGitPlanRejectsRetargetedRemoteBeforeCommit(t *testing.T) {
	workspace := t.TempDir()
	configuredRemote := filepath.Join(t.TempDir(), "configured.git")
	retargetedRemote := filepath.Join(t.TempDir(), "retargeted.git")
	for _, remote := range []string{configuredRemote, retargetedRemote} {
		if output, err := exec.Command("git", "init", "--bare", "-q", remote).CombinedOutput(); err != nil {
			t.Fatalf("initialize test remote: %v\n%s", err, output)
		}
	}
	for _, arguments := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "Git-BBQ Test"},
		{"config", "user.email", "test@example.invalid"},
		{"remote", "add", "origin", configuredRemote},
	} {
		if output, err := exec.Command("git", append([]string{"-C", workspace}, arguments...)...).CombinedOutput(); err != nil {
			t.Fatalf("initialize workspace Git state: %v\n%s", err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("baseline\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", workspace, "add", "README.md").CombinedOutput(); err != nil {
		t.Fatalf("stage test baseline: %v\n%s", err, output)
	}
	if output, err := exec.Command("git", "-C", workspace, "commit", "-qm", "chore: baseline").CombinedOutput(); err != nil {
		t.Fatalf("create test baseline: %v\n%s", err, output)
	}
	if err := os.WriteFile(filepath.Join(workspace, "feature.txt"), []byte("pending change\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", workspace, "add", "feature.txt").CombinedOutput(); err != nil {
		t.Fatalf("stage feature change: %v\n%s", err, output)
	}
	before, err := exec.Command("git", "-C", workspace, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}

	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	config.Remote.Status = "configured"
	config.Remote.URL = configuredRemote
	plan, err := PlanGitAction(config, GitActionCommit, GitPlanRequest{Message: "feat: guard push destination"})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Allowed || len(plan.FollowUpCommands) != 1 {
		t.Fatalf("commit was not planned with its required push: %#v", plan)
	}
	if output, err := exec.Command("git", "-C", workspace, "remote", "set-url", "origin", retargetedRemote).CombinedOutput(); err != nil {
		t.Fatalf("retarget workspace remote: %v\n%s", err, output)
	}

	if _, err := ExecuteGitPlan(config, workspace, plan, true, OSGitRunner{Timeout: time.Second}); err == nil {
		t.Fatal("execution accepted a remote alias that no longer matches GitHabits")
	} else {
		t.Logf("rejected retargeted remote as expected: %v", err)
	}
	after, err := exec.Command("git", "-C", workspace, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("commit ran before the changed push destination was rejected")
	}
	if _, err := exec.Command("git", "--git-dir", retargetedRemote, "show-ref", "--verify", "refs/heads/main").Output(); err == nil {
		t.Fatal("retargeted remote received an unexpected branch")
	}
}

func TestExecuteGitPlanCreatesAnnotatedSemverTag(t *testing.T) {
	workspace := t.TempDir()
	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	config.Push.AfterCommit = false
	config.Push.AfterTag = false
	runner := OSGitRunner{Timeout: time.Second}

	initPlan, err := PlanGitAction(config, GitActionInit, GitPlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, initPlan, true, runner); err != nil {
		t.Fatal(err)
	}
	branchPlan, err := PlanGitAction(config, GitActionBranch, GitPlanRequest{Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, branchPlan, true, runner); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(workspace, []string{"git", "config", "user.email", "git-bbq@example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(workspace, []string{"git", "config", "user.name", "Git BBQ Test"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("tag tracer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stagePlan, err := PlanGitAction(config, GitActionStage, GitPlanRequest{Paths: []string{"README.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, stagePlan, true, runner); err != nil {
		t.Fatal(err)
	}
	commitPlan, err := PlanGitAction(config, GitActionCommit, GitPlanRequest{Message: "feat: add tag tracer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, commitPlan, true, runner); err != nil {
		t.Fatal(err)
	}
	tagPlan, err := PlanGitAction(config, GitActionTag, GitPlanRequest{Tag: "v0.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGitPlan(config, workspace, tagPlan, true, runner); err != nil {
		t.Fatal(err)
	}

	tagType, err := exec.Command("git", "-C", workspace, "cat-file", "-t", "v0.1.0").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(tagType); got != "tag\n" {
		t.Fatalf("tag object type = %q", got)
	}
}
