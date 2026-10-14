package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/averyfreeman/git-bbq/internal/gitbbq"
)

type issueTargetOptions struct {
	workspace, owner, repo *string
	approved               *bool
}

func issueTargetFlags(fs *flag.FlagSet) issueTargetOptions {
	return issueTargetOptions{
		workspace: fs.String("workspace", "", "selected checkout; omitted for remote-only operation"),
		owner:     fs.String("owner", "", "explicit GitHub owner for remote-only operation"),
		repo:      fs.String("repo", "", "explicit GitHub repository for remote-only operation"),
		approved:  fs.Bool("approve", false, "approve the requested issue mutation"),
	}
}
func resolveIssueTarget(o issueTargetOptions) (string, string, error) {
	return gitbbq.ResolveIssueRepositoryTarget(*o.workspace, *o.owner, *o.repo)
}
func issueClient() (*gitbbq.GitHubIssueClient, context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	return gitbbq.NewGitHubIssueClient(), ctx, cancel
}
func requireIssueApproval(o issueTargetOptions) error {
	if !*o.approved {
		return errors.New("issue mutation requires --approve after workflow authorization")
	}
	return nil
}
func runIssues(args []string) error {
	if len(args) == 0 {
		return errors.New("issues requires list, get, create, edit, labels, assign, comment, close, reopen, subissue, or block")
	}
	switch args[0] {
	case "list":
		return runIssueList(args[1:])
	case "get":
		return runIssueGet(args[1:])
	case "create":
		return runIssueCreate(args[1:])
	case "edit":
		return runIssueEdit(args[1:])
	case "labels":
		return runIssueLabels(args[1:])
	case "assign":
		return runIssueAssign(args[1:])
	case "comment":
		return runIssueComment(args[1:])
	case "close":
		return runIssueState(args[1:], "closed")
	case "reopen":
		return runIssueState(args[1:], "open")
	case "subissue":
		return runIssueSubissue(args[1:])
	case "block":
		return runIssueBlock(args[1:])
	default:
		return fmt.Errorf("unsupported issues subcommand %q", args[0])
	}
}
func runIssueList(args []string) error {
	fs := flag.NewFlagSet("issues list", flag.ContinueOnError)
	target := issueTargetFlags(fs)
	state := fs.String("state", "open", "issue state (open|closed|all)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	owner, repo, err := resolveIssueTarget(target)
	if err != nil {
		return err
	}
	client, ctx, cancel := issueClient()
	defer cancel()
	result, err := client.ListIssues(ctx, owner, repo, *state)
	if err != nil {
		return err
	}
	return output(result, "json")
}
func runIssueGet(args []string) error {
	fs := flag.NewFlagSet("issues get", flag.ContinueOnError)
	target := issueTargetFlags(fs)
	number := fs.Int("number", 0, "issue number")
	if err := fs.Parse(args); err != nil {
		return err
	}
	owner, repo, err := resolveIssueTarget(target)
	if err != nil {
		return err
	}
	client, ctx, cancel := issueClient()
	defer cancel()
	result, err := client.GetIssue(ctx, owner, repo, *number)
	if err != nil {
		return err
	}
	return output(result, "json")
}
func runIssueCreate(args []string) error {
	fs := flag.NewFlagSet("issues create", flag.ContinueOnError)
	target := issueTargetFlags(fs)
	title := fs.String("title", "", "issue title")
	body := fs.String("body", "", "issue body")
	var labels, assignees stringList
	fs.Var(&labels, "label", "label to apply; repeat or comma-separate")
	fs.Var(&assignees, "assignee", "assignee login; repeat or comma-separate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireIssueApproval(target); err != nil {
		return err
	}
	owner, repo, err := resolveIssueTarget(target)
	if err != nil {
		return err
	}
	client, ctx, cancel := issueClient()
	defer cancel()
	result, err := client.CreateIssue(ctx, owner, repo, gitbbq.IssueCreateRequest{Title: *title, Body: *body, Labels: labels, Assignees: assignees})
	if err != nil {
		return err
	}
	return output(result, "json")
}
func runIssueEdit(args []string) error {
	fs := flag.NewFlagSet("issues edit", flag.ContinueOnError)
	target := issueTargetFlags(fs)
	number := fs.Int("number", 0, "issue number")
	title := fs.String("title", "", "replacement title")
	body := fs.String("body", "", "replacement body")
	state := fs.String("state", "", "new state (open|closed)")
	reason := fs.String("reason", "", "state reason (completed|not_planned)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireIssueApproval(target); err != nil {
		return err
	}
	request := gitbbq.IssueUpdateRequest{}
	fs.Visit(func(item *flag.Flag) {
		switch item.Name {
		case "title":
			request.Title = title
		case "body":
			request.Body = body
		case "state":
			request.State = state
		case "reason":
			request.StateReason = reason
		}
	})
	owner, repo, err := resolveIssueTarget(target)
	if err != nil {
		return err
	}
	client, ctx, cancel := issueClient()
	defer cancel()
	result, err := client.EditIssue(ctx, owner, repo, *number, request)
	if err != nil {
		return err
	}
	return output(result, "json")
}
func runIssueLabels(args []string) error {
	fs := flag.NewFlagSet("issues labels", flag.ContinueOnError)
	target := issueTargetFlags(fs)
	number := fs.Int("number", 0, "issue number")
	var add, remove stringList
	fs.Var(&add, "add", "label to add")
	fs.Var(&remove, "remove", "label to remove")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireIssueApproval(target); err != nil {
		return err
	}
	if (len(add) == 0) == (len(remove) == 0) {
		return errors.New("provide either --add or --remove labels, not both")
	}
	owner, repo, err := resolveIssueTarget(target)
	if err != nil {
		return err
	}
	client, ctx, cancel := issueClient()
	defer cancel()
	var result []gitbbq.IssueLabel
	if len(add) > 0 {
		result, err = client.AddIssueLabels(ctx, owner, repo, *number, add)
	} else {
		result, err = client.RemoveIssueLabels(ctx, owner, repo, *number, remove)
	}
	if err != nil {
		return err
	}
	return output(result, "json")
}
func runIssueAssign(args []string) error {
	fs := flag.NewFlagSet("issues assign", flag.ContinueOnError)
	target := issueTargetFlags(fs)
	number := fs.Int("number", 0, "issue number")
	var assignees stringList
	fs.Var(&assignees, "assignee", "assignee login; repeat or comma-separate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireIssueApproval(target); err != nil {
		return err
	}
	owner, repo, err := resolveIssueTarget(target)
	if err != nil {
		return err
	}
	client, ctx, cancel := issueClient()
	defer cancel()
	result, err := client.AssignIssue(ctx, owner, repo, *number, assignees)
	if err != nil {
		return err
	}
	return output(result, "json")
}
func runIssueComment(args []string) error {
	fs := flag.NewFlagSet("issues comment", flag.ContinueOnError)
	target := issueTargetFlags(fs)
	number := fs.Int("number", 0, "issue number")
	body := fs.String("body", "", "comment body")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireIssueApproval(target); err != nil {
		return err
	}
	owner, repo, err := resolveIssueTarget(target)
	if err != nil {
		return err
	}
	client, ctx, cancel := issueClient()
	defer cancel()
	result, err := client.CommentIssue(ctx, owner, repo, *number, *body)
	if err != nil {
		return err
	}
	return output(result, "json")
}
func runIssueState(args []string, value string) error {
	fs := flag.NewFlagSet("issues state", flag.ContinueOnError)
	target := issueTargetFlags(fs)
	number := fs.Int("number", 0, "issue number")
	reason := fs.String("reason", "", "close reason (completed|not_planned)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireIssueApproval(target); err != nil {
		return err
	}
	owner, repo, err := resolveIssueTarget(target)
	if err != nil {
		return err
	}
	client, ctx, cancel := issueClient()
	defer cancel()
	request := gitbbq.IssueUpdateRequest{State: &value}
	if *reason != "" {
		request.StateReason = reason
	}
	result, err := client.EditIssue(ctx, owner, repo, *number, request)
	if err != nil {
		return err
	}
	return output(result, "json")
}
func runIssueSubissue(args []string) error {
	fs := flag.NewFlagSet("issues subissue", flag.ContinueOnError)
	target := issueTargetFlags(fs)
	parent := fs.Int("parent", 0, "parent issue number")
	child := fs.Int("child", 0, "child issue number")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireIssueApproval(target); err != nil {
		return err
	}
	owner, repo, err := resolveIssueTarget(target)
	if err != nil {
		return err
	}
	client, ctx, cancel := issueClient()
	defer cancel()
	if err := client.LinkSubIssue(ctx, owner, repo, *parent, *child); err != nil {
		return err
	}
	return output(map[string]any{"parent": *parent, "child": *child, "linked": true}, "json")
}
func runIssueBlock(args []string) error {
	fs := flag.NewFlagSet("issues block", flag.ContinueOnError)
	target := issueTargetFlags(fs)
	issue := fs.Int("issue", 0, "blocked issue number")
	blocker := fs.Int("by", 0, "blocking issue number")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireIssueApproval(target); err != nil {
		return err
	}
	owner, repo, err := resolveIssueTarget(target)
	if err != nil {
		return err
	}
	client, ctx, cancel := issueClient()
	defer cancel()
	if err := client.AddIssueBlocker(ctx, owner, repo, *issue, *blocker); err != nil {
		return err
	}
	return output(map[string]any{"issue": *issue, "blocked_by": *blocker, "linked": true}, "json")
}
