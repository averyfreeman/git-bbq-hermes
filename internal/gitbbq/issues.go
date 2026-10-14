package gitbbq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const defaultIssueTriageLabel = "needs-triage"
const maxGitHubResponseBytes = 10 << 20

var githubRepositoryPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var scpGitHubRemote = regexp.MustCompile(`^(?:git@|ssh://git@)github\.com[:/]([^/]+)/(.+?)(?:\.git)?$`)

// GitHubIssue is the subset of the GitHub issue response used by Git-BBQ.
type GitHubIssue struct {
	ID          int64            `json:"id"`
	Number      int              `json:"number"`
	Title       string           `json:"title"`
	Body        string           `json:"body"`
	State       string           `json:"state"`
	StateReason string           `json:"state_reason,omitempty"`
	URL         string           `json:"html_url"`
	Labels      []IssueLabel     `json:"labels,omitempty"`
	Assignees   []IssueAssignee  `json:"assignees,omitempty"`
	PullRequest *json.RawMessage `json:"pull_request,omitempty"`
}

// IssueLabel describes a repository label attached to an issue.
type IssueLabel struct {
	Name string `json:"name"`
}

// IssueAssignee contains the account login returned by GitHub.
type IssueAssignee struct {
	Login string `json:"login"`
}

// IssueComment is the subset of a comment response used by Git-BBQ.
type IssueComment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	URL  string `json:"html_url"`
}

// IssueCreateRequest describes a new GitHub issue.
type IssueCreateRequest struct {
	Title     string
	Body      string
	Labels    []string
	Assignees []string
}

// IssueUpdateRequest contains only fields selected for an issue update.
type IssueUpdateRequest struct {
	Title       *string
	Body        *string
	State       *string
	StateReason *string
}

// GitHubCommandRunner executes gh without exposing its credentials or stderr.
type GitHubCommandRunner func(context.Context, []string, []byte) ([]byte, error)

// GitHubIssueClient uses gh api so authentication remains owned by gh or the
// active Hermes profile's GH_TOKEN/GITHUB_TOKEN environment.
type GitHubIssueClient struct{ run GitHubCommandRunner }

// NewGitHubIssueClient constructs a client using the authenticated gh CLI.
func NewGitHubIssueClient() *GitHubIssueClient {
	return &GitHubIssueClient{run: runGitHubCLI}
}

// NewGitHubIssueClientWithRunner constructs a client with an injectable runner.
func NewGitHubIssueClientWithRunner(run GitHubCommandRunner) *GitHubIssueClient {
	return &GitHubIssueClient{run: run}
}

func runGitHubCLI(ctx context.Context, args []string, input []byte) ([]byte, error) {
	command := exec.CommandContext(ctx, "gh", args...)
	if len(input) > 0 {
		command.Stdin = bytes.NewReader(input)
	}
	stdout := &boundedBuffer{limit: maxGitHubResponseBytes}
	command.Stdout = stdout
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return nil, err
	}
	if stdout.exceeded {
		return nil, errors.New("GitHub API response exceeded size limit")
	}
	return stdout.buffer.Bytes(), nil
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (writer *boundedBuffer) Write(data []byte) (int, error) {
	remaining := writer.limit - writer.buffer.Len()
	if remaining <= 0 {
		writer.exceeded = true
		return len(data), nil
	}
	if len(data) > remaining {
		writer.exceeded = true
		_, _ = writer.buffer.Write(data[:remaining])
		return len(data), nil
	}
	return writer.buffer.Write(data)
}

func (client *GitHubIssueClient) request(ctx context.Context, method, endpoint string, payload any, result any) error {
	if client == nil || client.run == nil {
		return errors.New("GitHub issue client is not configured")
	}
	var input []byte
	if payload != nil {
		var err error
		input, err = json.Marshal(payload)
		if err != nil {
			return errors.New("could not encode GitHub issue request")
		}
	}
	args := []string{"api", "--hostname", "github.com", "--method", method}
	if payload != nil {
		args = append(args, "--input", "-")
	}
	args = append(args, endpoint)
	output, err := client.run(ctx, args, input)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("GitHub CLI is unavailable; install gh and authenticate it or provide GH_TOKEN/GITHUB_TOKEN in the active Hermes profile environment")
		}
		return errors.New("GitHub API request failed; check repository access and issue permissions")
	}
	if result == nil || len(bytes.TrimSpace(output)) == 0 {
		return nil
	}
	if len(secretKindsOnLine(string(output))) > 0 {
		return errors.New("GitHub response contains credential-like data and was withheld")
	}
	if err := json.Unmarshal(output, result); err != nil {
		return errors.New("GitHub API returned invalid JSON")
	}
	return nil
}

// ResolveIssueRepositoryTarget uses the selected checkout's origin when present.
// Without a checkout it requires an explicit owner/repository pair and never
// infers a repository from the process working directory.
func ResolveIssueRepositoryTarget(workspace, owner, repository string) (string, string, error) {
	workspace, owner, repository = strings.TrimSpace(workspace), strings.TrimSpace(owner), strings.TrimSpace(repository)
	if workspace == "" {
		if err := validateIssueRepository(owner, repository); err != nil {
			return "", "", err
		}
		return owner, repository, nil
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", errors.New("could not resolve selected workspace")
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return "", "", errors.New("selected workspace must be an existing directory")
	}
	output, err := exec.Command("git", "-C", absolute, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", "", errors.New("selected workspace must have a GitHub origin remote")
	}
	originOwner, originRepository, err := parseGitHubRepositoryRemote(strings.TrimSpace(string(output)))
	if err != nil {
		return "", "", err
	}
	if owner != "" || repository != "" {
		if owner == "" || repository == "" || !strings.EqualFold(owner, originOwner) || !strings.EqualFold(repository, originRepository) {
			return "", "", errors.New("explicit issue repository conflicts with the selected workspace origin")
		}
	}
	return originOwner, originRepository, nil
}

func parseGitHubRepositoryRemote(remote string) (string, string, error) {
	if match := scpGitHubRemote.FindStringSubmatch(remote); len(match) == 3 {
		owner, repository := match[1], strings.TrimSuffix(match[2], ".git")
		if err := validateIssueRepository(owner, repository); err != nil {
			return "", "", err
		}
		return owner, repository, nil
	}
	parsed, err := url.Parse(remote)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !strings.EqualFold(parsed.Hostname(), "github.com") || (parsed.Scheme != "https" && parsed.Scheme != "http" && parsed.Scheme != "ssh") {
		return "", "", errors.New("selected workspace origin must be a credential-free GitHub repository URL")
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	if len(parts) != 2 {
		return "", "", errors.New("selected workspace origin must identify one GitHub owner and repository")
	}
	if err := validateIssueRepository(parts[0], parts[1]); err != nil {
		return "", "", err
	}
	return parts[0], parts[1], nil
}

func validateIssueRepository(owner, repository string) error {
	if !githubRepositoryPart.MatchString(owner) || !githubRepositoryPart.MatchString(repository) || owner == "." || owner == ".." || repository == "." || repository == ".." {
		return errors.New("a valid GitHub owner and repository are required")
	}
	return nil
}

func issueEndpoint(owner, repository, suffix string) (string, error) {
	if err := validateIssueRepository(owner, repository); err != nil {
		return "", err
	}
	endpoint := "repos/" + owner + "/" + repository + "/issues"
	if suffix != "" {
		endpoint += "/" + suffix
	}
	return endpoint, nil
}

func validateIssueContent(values ...string) error {
	for _, value := range values {
		for _, line := range strings.Split(value, "\n") {
			if len(secretKindsOnLine(line)) > 0 {
				return errors.New("issue content contains credential-like data")
			}
		}
	}
	return nil
}

func validateFetchedIssue(issue GitHubIssue) error {
	values := []string{issue.Title, issue.Body, issue.URL}
	for _, label := range issue.Labels {
		values = append(values, label.Name)
	}
	for _, assignee := range issue.Assignees {
		values = append(values, assignee.Login)
	}
	if err := validateIssueContent(values...); err != nil {
		return errors.New("GitHub issue contains credential-like content and was withheld")
	}
	return nil
}

func validateFetchedLabels(labels []IssueLabel) error {
	values := make([]string, 0, len(labels))
	for _, label := range labels {
		values = append(values, label.Name)
	}
	if err := validateIssueContent(values...); err != nil {
		return errors.New("GitHub issue labels contain credential-like content and were withheld")
	}
	return nil
}

// ListIssues returns up to 100 issues, excluding pull requests returned by GitHub.
func (client *GitHubIssueClient) ListIssues(ctx context.Context, owner, repository, state string) ([]GitHubIssue, error) {
	if state == "" {
		state = "open"
	}
	if state != "open" && state != "closed" && state != "all" {
		return nil, errors.New("issue state must be open, closed, or all")
	}
	endpoint, err := issueEndpoint(owner, repository, "")
	if err != nil {
		return nil, err
	}
	endpoint += "?state=" + url.QueryEscape(state) + "&per_page=100&page=1"
	var issues []GitHubIssue
	if err := client.request(ctx, "GET", endpoint, nil, &issues); err != nil {
		return nil, err
	}
	filtered := make([]GitHubIssue, 0, len(issues))
	for _, issue := range issues {
		if issue.PullRequest == nil {
			if err := validateFetchedIssue(issue); err != nil {
				return nil, err
			}
			filtered = append(filtered, issue)
		}
	}
	return filtered, nil
}

// GetIssue reads one issue by number.
func (client *GitHubIssueClient) GetIssue(ctx context.Context, owner, repository string, number int) (GitHubIssue, error) {
	if err := validateIssueNumber(number); err != nil {
		return GitHubIssue{}, err
	}
	endpoint, err := issueEndpoint(owner, repository, fmt.Sprint(number))
	if err != nil {
		return GitHubIssue{}, err
	}
	var issue GitHubIssue
	if err := client.request(ctx, "GET", endpoint, nil, &issue); err != nil {
		return GitHubIssue{}, err
	}
	if err := validateFetchedIssue(issue); err != nil {
		return GitHubIssue{}, err
	}
	return issue, nil
}

// CreateIssue creates an issue and applies needs-triage when labels are omitted.
func (client *GitHubIssueClient) CreateIssue(ctx context.Context, owner, repository string, request IssueCreateRequest) (GitHubIssue, error) {
	if strings.TrimSpace(request.Title) == "" {
		return GitHubIssue{}, errors.New("issue title is required")
	}
	if err := validateIssueContent(request.Title, request.Body); err != nil {
		return GitHubIssue{}, err
	}
	issueMetadata := append([]string(nil), request.Labels...)
	issueMetadata = append(issueMetadata, request.Assignees...)
	if err := validateIssueContent(issueMetadata...); err != nil {
		return GitHubIssue{}, err
	}
	if len(request.Labels) == 0 {
		request.Labels = []string{defaultIssueTriageLabel}
	}
	endpoint, err := issueEndpoint(owner, repository, "")
	if err != nil {
		return GitHubIssue{}, err
	}
	payload := map[string]any{"title": request.Title, "body": request.Body, "labels": request.Labels}
	if len(request.Assignees) > 0 {
		payload["assignees"] = request.Assignees
	}
	var issue GitHubIssue
	if err := client.request(ctx, "POST", endpoint, payload, &issue); err != nil {
		return GitHubIssue{}, err
	}
	if err := validateFetchedIssue(issue); err != nil {
		return GitHubIssue{}, err
	}
	return issue, nil
}

// EditIssue changes only fields supplied in request.
func (client *GitHubIssueClient) EditIssue(ctx context.Context, owner, repository string, number int, request IssueUpdateRequest) (GitHubIssue, error) {
	if err := validateIssueNumber(number); err != nil {
		return GitHubIssue{}, err
	}
	if request.Title == nil && request.Body == nil && request.State == nil && request.StateReason == nil {
		return GitHubIssue{}, errors.New("issue edit requires at least one changed field")
	}
	content := []string{}
	if request.Title != nil {
		content = append(content, *request.Title)
	}
	if request.Body != nil {
		content = append(content, *request.Body)
	}
	if err := validateIssueContent(content...); err != nil {
		return GitHubIssue{}, err
	}
	if request.State != nil && *request.State != "open" && *request.State != "closed" {
		return GitHubIssue{}, errors.New("issue state must be open or closed")
	}
	if request.StateReason != nil && *request.StateReason != "completed" && *request.StateReason != "not_planned" {
		return GitHubIssue{}, errors.New("issue close reason must be completed or not_planned")
	}
	if request.StateReason != nil && (request.State == nil || *request.State != "closed") {
		return GitHubIssue{}, errors.New("issue close reason requires state=closed")
	}
	endpoint, err := issueEndpoint(owner, repository, fmt.Sprint(number))
	if err != nil {
		return GitHubIssue{}, err
	}
	payload := map[string]any{}
	if request.Title != nil {
		payload["title"] = *request.Title
	}
	if request.Body != nil {
		payload["body"] = *request.Body
	}
	if request.State != nil {
		payload["state"] = *request.State
	}
	if request.StateReason != nil {
		payload["state_reason"] = *request.StateReason
	}
	var issue GitHubIssue
	if err := client.request(ctx, "PATCH", endpoint, payload, &issue); err != nil {
		return GitHubIssue{}, err
	}
	if err := validateFetchedIssue(issue); err != nil {
		return GitHubIssue{}, err
	}
	return issue, nil
}

// AddIssueLabels adds labels without replacing existing ones.
func (client *GitHubIssueClient) AddIssueLabels(ctx context.Context, owner, repository string, number int, labels []string) ([]IssueLabel, error) {
	if err := validateIssueNumber(number); err != nil {
		return nil, err
	}
	if len(labels) == 0 {
		return nil, errors.New("at least one label is required")
	}
	if err := validateIssueContent(labels...); err != nil {
		return nil, err
	}
	endpoint, err := issueEndpoint(owner, repository, fmt.Sprintf("%d/labels", number))
	if err != nil {
		return nil, err
	}
	var result []IssueLabel
	if err := client.request(ctx, "POST", endpoint, map[string]any{"labels": labels}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// RemoveIssueLabels removes only the named labels.
func (client *GitHubIssueClient) RemoveIssueLabels(ctx context.Context, owner, repository string, number int, labels []string) ([]IssueLabel, error) {
	if err := validateIssueNumber(number); err != nil {
		return nil, err
	}
	if len(labels) == 0 {
		return nil, errors.New("at least one label is required")
	}
	if err := validateIssueContent(labels...); err != nil {
		return nil, err
	}
	for _, label := range labels {
		endpoint, err := issueEndpoint(owner, repository, fmt.Sprintf("%d/labels/%s", number, url.PathEscape(label)))
		if err != nil {
			return nil, err
		}
		if err := client.request(ctx, "DELETE", endpoint, nil, nil); err != nil {
			return nil, err
		}
	}
	return client.GetIssueLabels(ctx, owner, repository, number)
}

// GetIssueLabels returns the labels currently attached to an issue.
func (client *GitHubIssueClient) GetIssueLabels(ctx context.Context, owner, repository string, number int) ([]IssueLabel, error) {
	if err := validateIssueNumber(number); err != nil {
		return nil, err
	}
	endpoint, err := issueEndpoint(owner, repository, fmt.Sprintf("%d/labels", number))
	if err != nil {
		return nil, err
	}
	var labels []IssueLabel
	if err := client.request(ctx, "GET", endpoint, nil, &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// AssignIssue adds one or more assignees to an issue.
func (client *GitHubIssueClient) AssignIssue(ctx context.Context, owner, repository string, number int, assignees []string) ([]IssueAssignee, error) {
	if err := validateIssueNumber(number); err != nil {
		return nil, err
	}
	if len(assignees) == 0 {
		return nil, errors.New("at least one assignee is required")
	}
	if err := validateIssueContent(assignees...); err != nil {
		return nil, err
	}
	endpoint, err := issueEndpoint(owner, repository, fmt.Sprintf("%d/assignees", number))
	if err != nil {
		return nil, err
	}
	var result struct {
		Assignees []IssueAssignee `json:"assignees"`
	}
	if err := client.request(ctx, "POST", endpoint, map[string]any{"assignees": assignees}, &result); err != nil {
		return nil, err
	}
	return result.Assignees, nil
}

// CommentIssue adds a comment to an issue.
func (client *GitHubIssueClient) CommentIssue(ctx context.Context, owner, repository string, number int, body string) (IssueComment, error) {
	if err := validateIssueNumber(number); err != nil {
		return IssueComment{}, err
	}
	if strings.TrimSpace(body) == "" {
		return IssueComment{}, errors.New("comment body is required")
	}
	if err := validateIssueContent(body); err != nil {
		return IssueComment{}, err
	}
	endpoint, err := issueEndpoint(owner, repository, fmt.Sprintf("%d/comments", number))
	if err != nil {
		return IssueComment{}, err
	}
	var comment IssueComment
	if err := client.request(ctx, "POST", endpoint, map[string]any{"body": body}, &comment); err != nil {
		return IssueComment{}, err
	}
	return comment, nil
}

// LinkSubIssue records child as a GitHub native sub-issue of parent.
func (client *GitHubIssueClient) LinkSubIssue(ctx context.Context, owner, repository string, parent, child int) error {
	if err := validateIssueNumber(parent); err != nil {
		return err
	}
	if err := validateIssueNumber(child); err != nil {
		return err
	}
	if parent == child {
		return errors.New("an issue cannot be its own sub-issue")
	}
	childIssue, err := client.GetIssue(ctx, owner, repository, child)
	if err != nil {
		return err
	}
	endpoint, err := issueEndpoint(owner, repository, fmt.Sprintf("%d/sub_issues", parent))
	if err != nil {
		return err
	}
	return client.request(ctx, "POST", endpoint, map[string]any{"sub_issue_id": childIssue.ID}, nil)
}

// AddIssueBlocker records blocker as a native blocked-by dependency of issue.
func (client *GitHubIssueClient) AddIssueBlocker(ctx context.Context, owner, repository string, issue, blocker int) error {
	if err := validateIssueNumber(issue); err != nil {
		return err
	}
	if err := validateIssueNumber(blocker); err != nil {
		return err
	}
	if issue == blocker {
		return errors.New("an issue cannot block itself")
	}
	blockerIssue, err := client.GetIssue(ctx, owner, repository, blocker)
	if err != nil {
		return err
	}
	endpoint, err := issueEndpoint(owner, repository, fmt.Sprintf("%d/dependencies/blocked_by", issue))
	if err != nil {
		return err
	}
	return client.request(ctx, "POST", endpoint, map[string]any{"issue_id": blockerIssue.ID}, nil)
}

func validateIssueNumber(number int) error {
	if number < 1 {
		return errors.New("issue number must be positive")
	}
	return nil
}
