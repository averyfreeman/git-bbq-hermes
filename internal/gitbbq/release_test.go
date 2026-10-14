package gitbbq

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRecommendReleaseAppliesConventionalCommitMapping(t *testing.T) {
	config := DefaultGitHabits()
	cases := []struct {
		name     string
		messages []string
		bump     string
		nextTag  string
	}{
		{name: "breaking change", messages: []string{"feat!: replace the storage API"}, bump: "major", nextTag: "v1.0.0"},
		{name: "custom breaking type", messages: []string{"security!: rotate exposed credentials"}, bump: "major", nextTag: "v1.0.0"},
		{name: "breaking footer", messages: []string{"feat: replace the storage API\n\nBREAKING CHANGE: response fields changed"}, bump: "major", nextTag: "v1.0.0"},
		{name: "feature", messages: []string{"feat: add project export"}, bump: "minor", nextTag: "v0.2.0"},
		{name: "fix and performance", messages: []string{"fix: handle empty path", "perf: reduce scan allocations"}, bump: "patch", nextTag: "v0.1.1"},
		{name: "no release", messages: []string{"docs: explain policy", "chore: update tooling", "test: add a fixture"}, bump: "none", nextTag: ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			plan, err := RecommendRelease(config, []string{"v0.1.0"}, testCase.messages)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Bump != testCase.bump {
				t.Fatalf("bump = %q, want %q", plan.Bump, testCase.bump)
			}
			if plan.NextTag != testCase.nextTag {
				t.Fatalf("next_tag = %q, want %q", plan.NextTag, testCase.nextTag)
			}
		})
	}
}

func TestRecommendReleaseRequiresExistingBaseline(t *testing.T) {
	if _, err := RecommendRelease(DefaultGitHabits(), nil, []string{"feat: add support"}); err == nil {
		t.Fatal("release was planned without an initial version baseline")
	}
}

func TestSafeSemanticVersionRejectsLeadingZeroNumericPrereleaseIdentifiers(t *testing.T) {
	for _, tag := range []string{"v1.2.3-01", "v1.2.3-alpha.02", "v1.2.3-00.1"} {
		if safeSemanticVersionTag(tag) {
			t.Fatalf("invalid SemVer tag %q was accepted", tag)
		}
	}
	for _, tag := range []string{"v1.2.3-0", "v1.2.3-alpha.1", "v1.2.3+01"} {
		if !safeSemanticVersionTag(tag) {
			t.Fatalf("valid SemVer tag %q was rejected", tag)
		}
	}
}

func TestPlanRepositoryReleaseReadsCommitsAfterLatestTag(t *testing.T) {
	workspace := t.TempDir()
	runReleaseTestGit(t, workspace, "init", "-q")
	runReleaseTestGit(t, workspace, "config", "user.name", "Git-BBQ Test")
	runReleaseTestGit(t, workspace, "config", "user.email", "test@example.invalid")
	file := filepath.Join(workspace, "history.txt")
	if err := os.WriteFile(file, []byte("baseline\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runReleaseTestGit(t, workspace, "add", "history.txt")
	runReleaseTestGit(t, workspace, "commit", "-qm", "chore: baseline")
	runReleaseTestGit(t, workspace, "tag", "v0.1.0")
	if err := os.WriteFile(file, []byte("feature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runReleaseTestGit(t, workspace, "add", "history.txt")
	runReleaseTestGit(t, workspace, "commit", "-qm", "feat: add feature")

	plan, err := PlanRepositoryRelease(workspace, DefaultGitHabits(), "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.CurrentTag != "v0.1.0" || plan.Bump != "minor" || plan.NextTag != "v0.2.0" {
		t.Fatalf("plan = %#v", plan)
	}
}

func runReleaseTestGit(t *testing.T, workspace string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", workspace}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", arguments, err, output)
	}
}
