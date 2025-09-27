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
	CreatePR(ctx context.Context, params CreateParams) error
	EditPRBase(ctx context.Context, number int, base string) error
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
}

// CreateParams describes the arguments used when opening a new pull request.
type CreateParams struct {
	Head string
	Base string
}

// PullRequest mirrors the fields consumed by the CLI presentation.
type PullRequest struct {
	ID             string `json:"id"`
	Number         int    `json:"number"`
	State          string `json:"state"`
	URL            string `json:"url"`
	Title          string `json:"title"`
	BaseRef        string `json:"baseRefName"`
	HeadRef        string `json:"headRefName"`
	Mergeable      string `json:"mergeable"`
	UpdatedAt      string `json:"updatedAt"`
	CreatedAt      string `json:"createdAt"`
	ReviewDecision string `json:"reviewDecision"`
	IsDraft        bool   `json:"isDraft"`
	Author         struct {
		Login string `json:"login"`
	} `json:"author"`
	ReviewRequests []struct {
		Login string `json:"login"`
	} `json:"reviewRequests"`
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
	args := []string{"pr", "list", "--json", strings.Join(listFields(), ",")}

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
		"updatedAt",
		"createdAt",
		"reviewDecision",
		"isDraft",
		"author.login",
		"reviewRequests[].login",
	}
}
