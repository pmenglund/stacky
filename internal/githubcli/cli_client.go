package githubcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type cliClient struct {
	bin string
}

func newCLIClient() (Client, error) {
	bin := strings.TrimSpace(os.Getenv("STACKY_GH_BIN"))
	if bin == "" {
		bin = "gh"
	}

	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("githubcli: gh executable not found: %w", err)
	}

	return &cliClient{bin: bin}, nil
}

func (c *cliClient) ListPRs(ctx context.Context, params ListParams) ([]PullRequest, error) {
	args := []string{"pr", "list"}

	if state := strings.TrimSpace(params.State); state != "" {
		args = append(args, "--state", strings.ToLower(state))
	}
	if author := strings.TrimSpace(params.Author); author != "" {
		args = append(args, "--author", author)
	}
	if head := strings.TrimSpace(params.Head); head != "" {
		args = append(args, "--head", head)
	}
	if search := strings.TrimSpace(params.Search); search != "" {
		args = append(args, "--search", search)
	}

	fields := cliFields(params.Fields)
	args = append(args, "--json", strings.Join(fields, ","))

	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var prs []PullRequest
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("githubcli: decode pr list: %w", err)
	}
	return prs, nil
}

func (c *cliClient) GetPRForBranch(ctx context.Context, params GetPRParams) (BranchPullRequests, error) {
	branch := strings.TrimSpace(params.Branch)
	if branch == "" {
		return BranchPullRequests{}, fmt.Errorf("githubcli: branch required")
	}

	prs, err := c.ListPRs(ctx, ListParams{Head: branch, State: "all", Fields: params.Fields})
	if err != nil {
		return BranchPullRequests{}, err
	}

	all := make(map[string]PullRequest, len(prs))
	var open *PullRequest
	for _, pr := range prs {
		if pr.ID != "" {
			all[pr.ID] = pr
		}
		state := strings.TrimSpace(pr.State)
		if state == "" {
			state = "OPEN"
		}
		if strings.EqualFold(state, "OPEN") {
			if open != nil {
				return BranchPullRequests{}, fmt.Errorf("githubcli: branch %s has multiple open pull requests", branch)
			}
			prCopy := pr
			open = &prCopy
		}
	}

	return BranchPullRequests{All: all, Open: open}, nil
}

func (c *cliClient) UpdateReviewers(ctx context.Context, params UpdateReviewersParams) error {
	if params.Number <= 0 {
		return fmt.Errorf("githubcli: invalid pull request number %d", params.Number)
	}

	add := normalizeReviewers(params.Add)
	remove := normalizeReviewers(params.Remove)

	if len(add) == 0 && len(remove) == 0 {
		return nil
	}

	number := fmt.Sprintf("%d", params.Number)

	if len(add) > 0 {
		args := []string{"pr", "reviewers", "add", number}
		for _, reviewer := range add {
			args = append(args, "--reviewer", reviewer)
		}
		if _, err := c.run(ctx, args...); err != nil {
			return err
		}
	}

	if len(remove) > 0 {
		args := []string{"pr", "reviewers", "remove", number}
		for _, reviewer := range remove {
			args = append(args, "--reviewer", reviewer)
		}
		if _, err := c.run(ctx, args...); err != nil {
			return err
		}
	}

	return nil
}

func (c *cliClient) CreatePR(ctx context.Context, params CreateParams) error {
	head := strings.TrimSpace(params.Head)
	base := strings.TrimSpace(params.Base)
	if head == "" {
		return fmt.Errorf("githubcli: head branch required")
	}
	if base == "" {
		return fmt.Errorf("githubcli: base branch required")
	}

	args := []string{"pr", "create", "--head", head, "--base", base, "--title", head, "--body", ""}
	_, err := c.run(ctx, args...)
	return err
}

func (c *cliClient) EditPRBase(ctx context.Context, number int, base string) error {
	if number <= 0 {
		return fmt.Errorf("githubcli: invalid pull request number %d", number)
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return fmt.Errorf("githubcli: base branch required")
	}

	args := []string{"pr", "edit", fmt.Sprintf("%d", number), "--base", base}
	_, err := c.run(ctx, args...)
	return err
}

func (c *cliClient) EditPRBody(ctx context.Context, number int, body string) error {
	if number <= 0 {
		return fmt.Errorf("githubcli: invalid pull request number %d", number)
	}

	args := []string{"pr", "edit", fmt.Sprintf("%d", number), "--body", body}
	_, err := c.run(ctx, args...)
	return err
}

func (c *cliClient) MergePR(ctx context.Context, params MergeParams) error {
	head := strings.TrimSpace(params.Head)
	sha := strings.TrimSpace(params.MatchHead)
	if head == "" {
		return fmt.Errorf("githubcli: head branch required")
	}
	if sha == "" {
		return fmt.Errorf("githubcli: head commit required")
	}

	branchPRs, err := c.GetPRForBranch(ctx, GetPRParams{Branch: head})
	if err != nil {
		return err
	}
	if branchPRs.Open == nil {
		return fmt.Errorf("githubcli: branch %s does not have an open pull request", head)
	}

	args := []string{"pr", "merge", head, "--squash", "--match-head-commit", sha}
	if params.Auto {
		args = append(args, "--auto")
	}

	_, err = c.run(ctx, args...)
	return err
}

func (c *cliClient) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Env = os.Environ()

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("githubcli: gh %s: %w: %s", strings.Join(args, " "), err, msg)
		}
		return nil, fmt.Errorf("githubcli: gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

var baseCLIFields = []string{
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
	"author",
	"reviewRequests",
	"statusCheckRollup",
	"body",
}

func cliFields(requested []string) []string {
	includeCommits := containsField(requested, "commits")
	uniq := make(map[string]struct{}, len(baseCLIFields)+len(requested)+1)
	fields := make([]string, 0, len(baseCLIFields)+len(requested)+1)

	for _, field := range baseCLIFields {
		lower := strings.ToLower(field)
		if _, ok := uniq[lower]; ok {
			continue
		}
		uniq[lower] = struct{}{}
		fields = append(fields, field)
	}

	if includeCommits {
		uniq["commits"] = struct{}{}
		fields = append(fields, "commits")
	}

	for _, field := range requested {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		lower := strings.ToLower(field)
		if _, ok := uniq[lower]; ok {
			continue
		}
		uniq[lower] = struct{}{}
		fields = append(fields, field)
	}

	return fields
}
