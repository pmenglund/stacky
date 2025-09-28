package githubcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Client describes the interactions stacky performs with the GitHub CLI.
type Client interface {
	ListPRs(ctx context.Context, params ListParams) ([]PullRequest, error)
	GetPRForBranch(ctx context.Context, params GetPRParams) (BranchPullRequests, error)
	UpdateReviewers(ctx context.Context, params UpdateReviewersParams) error
	CreatePR(ctx context.Context, params CreateParams) error
	EditPRBase(ctx context.Context, number int, base string) error
	EditPRBody(ctx context.Context, number int, body string) error
	MergePR(ctx context.Context, params MergeParams) error
}

// Runner abstracts execution of the `gh` binary to enable testing without
// invoking external processes.
type Runner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

type execRunner struct{}

func (r *execRunner) Run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Env = append(os.Environ(), "GH_PAGER=cat")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("githubcli: gh %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

// ListParams captures high-level filters used when listing PRs.
type ListParams struct {
	Author string
	Head   string
	State  string
	Search string
	Fields []string
}

// CreateParams describes the arguments used when opening a new pull request.
type CreateParams struct {
	Head string
	Base string
}

// MergeParams describes how to merge a pull request via the GitHub CLI.
type MergeParams struct {
	Head      string
	MatchHead string
	Auto      bool
}

// GetPRParams controls how branch-specific pull requests are fetched.
type GetPRParams struct {
	Branch string
	Fields []string
}

// BranchPullRequests captures all pull requests for a branch and the single open PR.
type BranchPullRequests struct {
	All  map[string]PullRequest
	Open *PullRequest
}

// UpdateReviewersParams describes reviewer modifications for an existing PR.
type UpdateReviewersParams struct {
	Number int
	Add    []string
	Remove []string
}

// PullRequest mirrors the fields consumed by the CLI presentation.
type PullRequest struct {
	ID               string `json:"id"`
	Number           int    `json:"number"`
	State            string `json:"state"`
	URL              string `json:"url"`
	Title            string `json:"title"`
	BaseRef          string `json:"baseRefName"`
	HeadRef          string `json:"headRefName"`
	Mergeable        string `json:"mergeable"`
	MergeStateStatus string `json:"mergeStateStatus"`
	UpdatedAt        string `json:"updatedAt"`
	CreatedAt        string `json:"createdAt"`
	ReviewDecision   string `json:"reviewDecision"`
	IsDraft          bool   `json:"isDraft"`
	Author           struct {
		Login string `json:"login"`
	} `json:"author"`
	ReviewRequests []struct {
		Login string `json:"login"`
	} `json:"reviewRequests"`
	StatusCheckRollup []struct {
		State string `json:"state"`
	} `json:"statusCheckRollup"`
	Body    string              `json:"body"`
	Commits []PullRequestCommit `json:"commits"`
}

// PullRequestCommit captures the subset of commit metadata required by stacky.
type PullRequestCommit struct {
	OID string `json:"oid"`
}

type client struct {
	runner Runner
}

// New constructs a client using the system `gh` command.
func New() Client {
	return &client{runner: &execRunner{}}
}

// NewWithRunner allows tests to supply a fake runner.
func NewWithRunner(r Runner) Client {
	return &client{runner: r}
}

func (c *client) ListPRs(ctx context.Context, params ListParams) ([]PullRequest, error) {
	fields := mergeJSONFields(listFields(), params.Fields)
	args := []string{"pr", "list", "--json", strings.Join(fields, ",")}

	if params.State != "" {
		args = append(args, "--state", params.State)
	}
	if params.Author != "" {
		args = append(args, "--author", params.Author)
	}
	if params.Head != "" {
		args = append(args, "--head", params.Head)
	}
	if params.Search != "" {
		args = append(args, "--search", params.Search)
	}

	out, err := c.runner.Run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var prs []PullRequest
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, fmt.Errorf("githubcli: decode gh output: %w", err)
	}

	return prs, nil
}

func (c *client) GetPRForBranch(ctx context.Context, params GetPRParams) (BranchPullRequests, error) {
	branch := strings.TrimSpace(params.Branch)
	if branch == "" {
		return BranchPullRequests{}, fmt.Errorf("githubcli: branch required")
	}

	fields := mergeJSONFields(listFields(), params.Fields)
	args := []string{"pr", "list", "--json", strings.Join(fields, ","), "--state", "all", "--head", branch}

	out, err := c.runner.Run(ctx, args...)
	if err != nil {
		return BranchPullRequests{}, err
	}

	var raw []PullRequest
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return BranchPullRequests{}, fmt.Errorf("githubcli: decode gh output: %w", err)
	}

	all := make(map[string]PullRequest, len(raw))
	var open *PullRequest
	for i := range raw {
		pr := raw[i]
		if pr.ID != "" {
			all[pr.ID] = pr
		}
		if strings.EqualFold(pr.State, "OPEN") {
			if open != nil {
				return BranchPullRequests{}, fmt.Errorf("githubcli: branch %s has multiple open pull requests", branch)
			}
			prCopy := pr
			open = &prCopy
		}
	}

	return BranchPullRequests{All: all, Open: open}, nil
}

func (c *client) UpdateReviewers(ctx context.Context, params UpdateReviewersParams) error {
	if params.Number <= 0 {
		return fmt.Errorf("githubcli: invalid pull request number %d", params.Number)
	}

	args := []string{"pr", "edit", fmt.Sprintf("%d", params.Number)}

	add := normalizeReviewers(params.Add)
	for _, reviewer := range add {
		args = append(args, "--add-reviewer", reviewer)
	}

	remove := normalizeReviewers(params.Remove)
	for _, reviewer := range remove {
		args = append(args, "--remove-reviewer", reviewer)
	}

	if len(args) == 3 {
		return nil
	}

	_, err := c.runner.Run(ctx, args...)
	return err
}

func (c *client) CreatePR(ctx context.Context, params CreateParams) error {
	if params.Head == "" {
		return fmt.Errorf("githubcli: head branch required")
	}
	if params.Base == "" {
		return fmt.Errorf("githubcli: base branch required")
	}
	args := []string{"pr", "create", "--head", params.Head, "--base", params.Base, "--fill"}
	_, err := c.runner.Run(ctx, args...)
	return err
}

func (c *client) EditPRBase(ctx context.Context, number int, base string) error {
	if number <= 0 {
		return fmt.Errorf("githubcli: invalid pull request number %d", number)
	}
	if base == "" {
		return fmt.Errorf("githubcli: base branch required")
	}
	args := []string{"pr", "edit", fmt.Sprintf("%d", number), "--base", base}
	_, err := c.runner.Run(ctx, args...)
	return err
}

func (c *client) EditPRBody(ctx context.Context, number int, body string) error {
	if number <= 0 {
		return fmt.Errorf("githubcli: invalid pull request number %d", number)
	}
	args := []string{"pr", "edit", fmt.Sprintf("%d", number), "--body", body}
	_, err := c.runner.Run(ctx, args...)
	return err
}

func (c *client) MergePR(ctx context.Context, params MergeParams) error {
	if params.Head == "" {
		return fmt.Errorf("githubcli: head branch required")
	}
	if params.MatchHead == "" {
		return fmt.Errorf("githubcli: head commit required")
	}
	args := []string{"pr", "merge", params.Head, "--squash", "--match-head-commit", params.MatchHead}
	if params.Auto {
		args = append(args, "--auto")
	}
	_, err := c.runner.Run(ctx, args...)
	return err
}

func listFields() []string {
	return []string{
		"id",
		"number",
		"state",
		"url",
		"title",
		"baseRefName",
		"headRefName",
		"mergeable",
		"mergeStateStatus",
		"updatedAt",
		"createdAt",
		"reviewDecision",
		"isDraft",
		"author.login",
		"reviewRequests[].login",
		"statusCheckRollup",
		"body",
	}
}

func mergeJSONFields(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	result := make([]string, 0, len(base)+len(extra))
	for _, field := range base {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}
	for _, field := range extra {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}
	return result
}

func normalizeReviewers(reviewers []string) []string {
	if len(reviewers) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(reviewers))
	result := make([]string, 0, len(reviewers))
	for _, reviewer := range reviewers {
		reviewer = strings.TrimSpace(reviewer)
		if reviewer == "" {
			continue
		}
		if _, ok := seen[reviewer]; ok {
			continue
		}
		seen[reviewer] = struct{}{}
		result = append(result, reviewer)
	}
	return result
}
