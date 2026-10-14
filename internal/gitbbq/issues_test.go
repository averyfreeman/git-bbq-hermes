package gitbbq

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

type issueCall struct {
	args  []string
	input []byte
}
type fakeIssueRunner struct {
	calls   []issueCall
	respond func([]string, []byte) ([]byte, error)
}

func (r *fakeIssueRunner) run(_ context.Context, args []string, input []byte) ([]byte, error) {
	r.calls = append(r.calls, issueCall{append([]string(nil), args...), append([]byte(nil), input...)})
	if r.respond == nil {
		return []byte(`{}`), nil
	}
	return r.respond(args, input)
}

func TestResolveIssueRepositoryTarget(t *testing.T) {
	if _, _, err := ResolveIssueRepositoryTarget("", "", ""); err == nil {
		t.Fatal("missing remote-only target accepted")
	}
	owner, repo, err := ResolveIssueRepositoryTarget("", "example-org", "project")
	if err != nil || owner != "example-org" || repo != "project" {
		t.Fatalf("explicit target = %s/%s, %v", owner, repo, err)
	}
	for _, remote := range []string{"https://github.com/example-org/project.git", "git@github.com:example-org/project.git", "ssh://git@github.com/example-org/project.git"} {
		owner, repo, err := parseGitHubRepositoryRemote(remote)
		if err != nil || owner != "example-org" || repo != "project" {
			t.Fatalf("parse %q = %s/%s, %v", remote, owner, repo, err)
		}
	}
	if _, _, err := parseGitHubRepositoryRemote("https://user:secret@github.com/example-org/project.git"); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("credential origin error = %v", err)
	}
	workspace := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://github.com/example-org/project.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", workspace}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	owner, repo, err = ResolveIssueRepositoryTarget(workspace, "", "")
	if err != nil || owner != "example-org" || repo != "project" {
		t.Fatalf("workspace target = %s/%s, %v", owner, repo, err)
	}
	if _, _, err := ResolveIssueRepositoryTarget(workspace, "elsewhere", "project"); err == nil {
		t.Fatal("conflicting target accepted")
	}
}

func TestGitHubIssueListCreateAndTriageLabel(t *testing.T) {
	runner := &fakeIssueRunner{}
	runner.respond = func(args []string, _ []byte) ([]byte, error) {
		endpoint := args[len(args)-1]
		if strings.Contains(endpoint, "?state=") {
			return []byte(`[{"id":1,"number":1,"title":"issue"},{"id":2,"number":2,"pull_request":{"url":"/pull/2"}}]`), nil
		}
		return []byte(`{"id":10,"number":10,"title":"New"}`), nil
	}
	client := NewGitHubIssueClientWithRunner(runner.run)
	issues, err := client.ListIssues(context.Background(), "example", "project", "open")
	if err != nil || len(issues) != 1 || issues[0].Number != 1 {
		t.Fatalf("ListIssues = %#v, %v", issues, err)
	}
	created, err := client.CreateIssue(context.Background(), "example", "project", IssueCreateRequest{Title: "New"})
	if err != nil || created.Number != 10 {
		t.Fatalf("CreateIssue = %#v, %v", created, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(runner.calls[1].input, &payload); err != nil {
		t.Fatal(err)
	}
	labels, ok := payload["labels"].([]any)
	if !ok || len(labels) != 1 || labels[0] != defaultIssueTriageLabel {
		t.Fatalf("default triage labels = %#v", payload["labels"])
	}
	if got := runner.calls[1].args; len(got) != 8 || got[0] != "api" || got[1] != "--hostname" || got[2] != "github.com" || got[3] != "--method" || got[4] != "POST" || got[5] != "--input" || got[6] != "-" || got[7] != "repos/example/project/issues" {
		t.Fatalf("gh api args = %v", got)
	}
}

func TestGitHubIssueRelationshipsUseNativeEndpoints(t *testing.T) {
	runner := &fakeIssueRunner{}
	runner.respond = func(args []string, _ []byte) ([]byte, error) {
		endpoint := args[len(args)-1]
		if strings.HasSuffix(endpoint, "/issues/4") {
			return []byte(`{"id":404,"number":4}`), nil
		}
		if strings.HasSuffix(endpoint, "/issues/7") {
			return []byte(`{"id":707,"number":7}`), nil
		}
		return []byte(`{}`), nil
	}
	client := NewGitHubIssueClientWithRunner(runner.run)
	if err := client.LinkSubIssue(context.Background(), "example", "project", 3, 4); err != nil {
		t.Fatal(err)
	}
	if err := client.AddIssueBlocker(context.Background(), "example", "project", 6, 7); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 4 || !strings.HasSuffix(runner.calls[1].args[7], "/issues/3/sub_issues") || string(runner.calls[1].input) != `{"sub_issue_id":404}` {
		t.Fatalf("subissue calls = %#v", runner.calls)
	}
	if !strings.HasSuffix(runner.calls[3].args[7], "/issues/6/dependencies/blocked_by") || string(runner.calls[3].input) != `{"issue_id":707}` {
		t.Fatalf("blocker calls = %#v", runner.calls)
	}
}

func TestGitHubIssueContentAndErrorsDoNotEchoCredentialValues(t *testing.T) {
	calls := 0
	client := NewGitHubIssueClientWithRunner(func(context.Context, []string, []byte) ([]byte, error) {
		calls++
		return nil, fmt.Errorf("opaque credential in stderr")
	})
	credential := "ghp_" + strings.Repeat("A", 24)
	_, err := client.CreateIssue(context.Background(), "example", "project", IssueCreateRequest{Title: "token=" + credential})
	if err == nil || strings.Contains(err.Error(), credential) || calls != 0 {
		t.Fatalf("secret content result = %v, calls=%d", err, calls)
	}
	_, err = client.GetIssue(context.Background(), "example", "project", 1)
	if err == nil || strings.Contains(err.Error(), "opaque credential") || calls != 1 {
		t.Fatalf("sanitized error = %v, calls=%d", err, calls)
	}
}

func TestIssueLabelMutationsRejectCredentialLikeValuesBeforeRequest(t *testing.T) {
	calls := 0
	client := NewGitHubIssueClientWithRunner(func(context.Context, []string, []byte) ([]byte, error) {
		calls++
		return []byte(`[]`), nil
	})
	credential := "access_token=" + strings.Repeat("A", 24)
	for _, mutate := range []struct {
		name string
		call func() error
	}{
		{name: "add", call: func() error {
			_, err := client.AddIssueLabels(context.Background(), "example", "project", 1, []string{credential})
			return err
		}},
		{name: "remove", call: func() error {
			_, err := client.RemoveIssueLabels(context.Background(), "example", "project", 1, []string{credential})
			return err
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			err := mutate.call()
			if err == nil || strings.Contains(err.Error(), credential) {
				t.Fatalf("credential-like label result = %v", err)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("GitHub requests = %d, want 0", calls)
	}
}

func TestIssueAssignmentRejectsCredentialLikeValuesBeforeRequest(t *testing.T) {
	calls := 0
	client := NewGitHubIssueClientWithRunner(func(context.Context, []string, []byte) ([]byte, error) {
		calls++
		return []byte(`{"assignees":[]}`), nil
	})
	credential := "access_token=" + strings.Repeat("B", 24)
	_, err := client.AssignIssue(context.Background(), "example", "project", 1, []string{credential})
	if err == nil || strings.Contains(err.Error(), credential) || calls != 0 {
		t.Fatalf("credential-like assignee result = %v, calls=%d", err, calls)
	}
}

func TestGitHubIssueReadWithCredentialLikeDataIsWithheld(t *testing.T) {
	credential := "ghp_" + strings.Repeat("B", 24)
	client := NewGitHubIssueClientWithRunner(func(context.Context, []string, []byte) ([]byte, error) {
		return []byte(`{"number":1,"title":"issue","body":"token=` + credential + `"}`), nil
	})
	_, err := client.GetIssue(context.Background(), "example", "project", 1)
	if err == nil || strings.Contains(err.Error(), credential) || !strings.Contains(err.Error(), "withheld") {
		t.Fatalf("credential-bearing GitHub response result = %v", err)
	}
}
