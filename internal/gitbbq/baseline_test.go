package gitbbq

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecuteInitialBaselineDeletesOnlyImportedLocalTags(t *testing.T) {
	workspace, upstream, origin, config := setupBaselineRepository(t, false)
	plan, err := PlanInitialBaseline(config, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Allowed || !plan.RequiresApproval {
		t.Fatalf("baseline plan lacks policy/approval gates: %#v", plan)
	}
	if len(plan.LocalTagsToRemove) != 1 || plan.LocalTagsToRemove[0] != "v0.2.0" {
		t.Fatalf("local tags to remove = %#v", plan.LocalTagsToRemove)
	}

	if _, err := ExecuteInitialBaseline(config, workspace, plan, false, OSGitRunner{Timeout: time.Second}); err == nil {
		t.Fatal("baseline executed without explicit approval")
	}
	execution, err := ExecuteInitialBaseline(config, workspace, plan, true, OSGitRunner{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if !execution.Executed || execution.ExitCode != 0 {
		t.Fatalf("execution = %#v", execution)
	}
	if _, err := exec.Command("git", "-C", workspace, "show-ref", "--verify", "refs/tags/v0.2.0").Output(); err == nil {
		t.Fatal("imported local tag was not removed")
	}
	if _, err := exec.Command("git", "-C", workspace, "cat-file", "-t", "v0.1.0").Output(); err != nil {
		t.Fatalf("initial local tag missing: %v", err)
	}
	if _, err := exec.Command("git", "--git-dir", origin, "show-ref", "--verify", "refs/tags/v0.1.0").Output(); err != nil {
		t.Fatalf("initial tag was not pushed to origin: %v", err)
	}
	if _, err := exec.Command("git", "--git-dir", upstream, "show-ref", "--verify", "refs/tags/v0.2.0").Output(); err != nil {
		t.Fatalf("upstream tag was changed: %v", err)
	}
}

func TestPlanInitialBaselinePreservesPublishedRemoteTags(t *testing.T) {
	workspace, _, origin, config := setupBaselineRepository(t, true)
	plan, err := PlanInitialBaseline(config, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Allowed || len(plan.LocalTagsToRemove) != 0 || len(plan.RemoteTagsKept) != 1 || plan.RemoteTagsKept[0] != "v0.2.0" {
		t.Fatalf("baseline plan did not preserve the published tag: %#v", plan)
	}
	if len(plan.Commands) != 2 || strings.Join(plan.Commands[0], " ") != "git tag -a v0.1.0 -m v0.1.0" || strings.Join(plan.Commands[1], " ") != "git push origin refs/tags/v0.1.0" {
		t.Fatalf("baseline commands = %#v", plan.Commands)
	}
	if _, err := exec.Command("git", "-C", workspace, "show-ref", "--verify", "refs/tags/v0.2.0").Output(); err != nil {
		t.Fatalf("read-only plan changed local tags: %v", err)
	}
	if _, err := exec.Command("git", "--git-dir", origin, "show-ref", "--verify", "refs/tags/v0.2.0").Output(); err != nil {
		t.Fatalf("read-only plan changed published remote tag: %v", err)
	}
}

func TestExecuteInitialBaselineAddsNewTagWithoutRewritingPublishedTags(t *testing.T) {
	workspace, _, origin, config := setupBaselineRepository(t, true)
	plan, err := PlanInitialBaseline(config, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteInitialBaseline(config, workspace, plan, true, OSGitRunner{Timeout: time.Second}); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"v0.1.0", "v0.2.0"} {
		if _, err := exec.Command("git", "--git-dir", origin, "show-ref", "--verify", "refs/tags/"+tag).Output(); err != nil {
			t.Fatalf("remote tag %s was not retained/created: %v", tag, err)
		}
	}
	if _, err := exec.Command("git", "-C", workspace, "show-ref", "--verify", "refs/tags/v0.2.0").Output(); err != nil {
		t.Fatalf("published local tag was removed: %v", err)
	}
}

func TestExecuteInitialBaselinePreservesSameNameTagWithDifferentObject(t *testing.T) {
	workspace, upstream, _, config := setupBaselineRepository(t, false)
	if err := os.WriteFile(filepath.Join(workspace, "new.txt"), []byte("different target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitBaselineTest(t, workspace, "add", "new.txt")
	gitBaselineTest(t, workspace, "commit", "-qm", "fix: add a different target")
	gitBaselineTest(t, workspace, "tag", "-f", "v0.2.0", "HEAD")

	localBefore, err := exec.Command("git", "-C", workspace, "rev-parse", "refs/tags/v0.2.0").Output()
	if err != nil {
		t.Fatal(err)
	}
	upstreamTag, err := exec.Command("git", "--git-dir", upstream, "rev-parse", "refs/tags/v0.2.0").Output()
	if err != nil {
		t.Fatal(err)
	}
	localObjectID := strings.TrimSpace(string(localBefore))
	if localObjectID == strings.TrimSpace(string(upstreamTag)) {
		t.Fatal("fixture did not create a tag object that differs from upstream")
	}

	plan, err := PlanInitialBaseline(config, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.LocalTagsToRemove) != 0 {
		t.Fatalf("conflicting same-name local tag was scheduled for deletion: %#v", plan.LocalTagsToRemove)
	}
	if _, err := ExecuteInitialBaseline(config, workspace, plan, true, OSGitRunner{Timeout: time.Second}); err != nil {
		t.Fatal(err)
	}
	localAfter, err := exec.Command("git", "-C", workspace, "rev-parse", "refs/tags/v0.2.0").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(localAfter)); got != localObjectID {
		t.Fatalf("local tag object changed from %s to %s", localObjectID, got)
	}
}

func setupBaselineRepository(t *testing.T, publishTag bool) (string, string, string, GitHabits) {
	t.Helper()
	workspace := t.TempDir()
	gitBaselineTest(t, workspace, "init", "-q", "-b", "main")
	gitBaselineTest(t, workspace, "config", "user.name", "Git-BBQ Test")
	gitBaselineTest(t, workspace, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("baseline\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitBaselineTest(t, workspace, "add", "README.md")
	gitBaselineTest(t, workspace, "commit", "-qm", "feat: baseline")
	gitBaselineTest(t, workspace, "tag", "v0.2.0")

	upstream := filepath.Join(t.TempDir(), "upstream.git")
	if output, err := exec.Command("git", "init", "--bare", "-q", upstream).CombinedOutput(); err != nil {
		t.Fatalf("initialize upstream bare remote: %v\n%s", err, output)
	}
	gitBaselineTest(t, workspace, "remote", "add", "upstream", upstream)
	gitBaselineTest(t, workspace, "push", "-q", "upstream", "HEAD:refs/heads/main")
	gitBaselineTest(t, workspace, "push", "-q", "upstream", "refs/tags/v0.2.0")

	origin := filepath.Join(t.TempDir(), "origin.git")
	if output, err := exec.Command("git", "init", "--bare", "-q", origin).CombinedOutput(); err != nil {
		t.Fatalf("initialize origin bare remote: %v\n%s", err, output)
	}
	gitBaselineTest(t, workspace, "remote", "add", "origin", origin)
	if publishTag {
		gitBaselineTest(t, workspace, "push", "-q", "origin", "refs/tags/v0.2.0")
	}

	config, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	config.Remote.Status = "configured"
	config.Remote.URL = origin
	return workspace, upstream, origin, config
}

func gitBaselineTest(t *testing.T, workspace string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", workspace}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", arguments, err, output)
	}
}
