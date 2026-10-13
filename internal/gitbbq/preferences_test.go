package gitbbq

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreferencesRoundTripUsesExplicitOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("GIT_BBQ_CONFIG", path)
	if err := SavePreferences(Preferences{Profile: "autonomous", Languages: []string{"go", "python"}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Profile != "autonomous" || len(loaded.Languages) != 2 {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestPreferencesRoundTripPreservesGitHabitsPreset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("GIT_BBQ_CONFIG", path)
	policy, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	policy.Branch = "feature/from-preset"
	if err := SavePreferences(Preferences{GitHabitsPreset: &policy, Languages: []string{"go"}}); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GitHabitsPreset == nil || loaded.Profile != "autonomous" || loaded.GitHabitsPreset.Version != 2 || loaded.GitHabitsPreset.Branch != "feature/from-preset" {
		t.Fatalf("loaded preferences = %#v", loaded)
	}
}

func TestSavePreferencesRejectsCredentialBearingPresetWithoutEchoingValue(t *testing.T) {
	t.Setenv("GIT_BBQ_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	policy, err := GitHabitsForProfile("autonomous")
	if err != nil {
		t.Fatal(err)
	}
	policy.Remote.Status = "configured"
	policy.Remote.URL = "https://agent:credential-value-123@example.com/project.git"

	err = SavePreferences(Preferences{GitHabitsPreset: &policy})
	if err == nil || !strings.Contains(err.Error(), "credential-like") {
		t.Fatalf("credential-bearing preset error = %v", err)
	}
	if strings.Contains(err.Error(), "credential-value-123") {
		t.Fatal("credential value was echoed in the validation error")
	}
}

func TestReadVersionOneGitHabitsPolicy(t *testing.T) {
	root := t.TempDir()
	policy := DefaultGitHabits()
	policy.Version = 1
	policy.Authorization = GitAuthorizationPolicy{}
	policy.Release = GitReleasePolicy{}
	policy.Push = GitPushPolicy{}
	policy.Remote.Visibility = "private"
	policy.Remote.VisibilityPolicy = ""

	data, err := marshalYAML(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, GitHabitsFilename), data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadGitHabits(root)
	if err != nil {
		t.Fatalf("read version-1 policy: %v", err)
	}
	if loaded.Version != 1 || loaded.Remote.Visibility != "private" {
		t.Fatalf("loaded version-1 policy = %#v", loaded)
	}
}
