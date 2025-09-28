package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/pmenglund/stacky/internal/githubcli"
)

// ImportAction describes the parent relationship that should be recorded for a branch.
type ImportAction struct {
	Branch       string
	Parent       string
	ParentCommit string
}

// ImportPlan summarizes the work required to import a Graphite-managed stack.
type ImportPlan struct {
	TopBranch  string
	BaseBranch string
	Actions    []ImportAction
	Lookups    []string
}

// PlanImport builds an ImportPlan for the specified branch by following GitHub PR metadata
// toward a known stack bottom.
func (e *Engine) PlanImport(ctx context.Context, top string) (ImportPlan, error) {
	top = strings.TrimSpace(top)
	if top == "" {
		return ImportPlan{}, fmt.Errorf("engine: branch name must not be empty")
	}

	ref := fmt.Sprintf("refs/heads/%s", top)
	head, err := e.repo.ReadRef(ctx, ref)
	if err != nil {
		return ImportPlan{}, err
	}
	if strings.TrimSpace(head) == "" {
		return ImportPlan{}, fmt.Errorf("engine: branch %s not found", top)
	}

	bottoms, err := e.stackBottomNames(ctx)
	if err != nil {
		return ImportPlan{}, err
	}

	plan := ImportPlan{TopBranch: top}
	visited := make(map[string]struct{})
	current := top

	for {
		if _, ok := bottoms[current]; ok {
			plan.BaseBranch = current
			break
		}

		if _, seen := visited[current]; seen {
			return ImportPlan{}, fmt.Errorf("engine: detected cycle when importing branch %s", current)
		}
		visited[current] = struct{}{}

		prs, err := e.gh.ListPRs(ctx, githubcli.ListParams{State: "all", Head: current, Fields: []string{"commits"}})
		if err != nil {
			return ImportPlan{}, err
		}

		var open []githubcli.PullRequest
		for _, pr := range prs {
			if strings.EqualFold(pr.State, "OPEN") {
				open = append(open, pr)
			}
		}

		if len(open) == 0 {
			return ImportPlan{}, fmt.Errorf("engine: branch %s has no open pull request", current)
		}
		if len(open) > 1 {
			return ImportPlan{}, fmt.Errorf("engine: branch %s has multiple open pull requests", current)
		}

		pr := open[0]
		headRef := strings.TrimSpace(pr.HeadRef)
		if headRef != "" && !strings.EqualFold(headRef, current) {
			return ImportPlan{}, fmt.Errorf("engine: pull request #%d head %s does not match branch %s", pr.Number, pr.HeadRef, current)
		}
		if len(pr.Commits) == 0 {
			return ImportPlan{}, fmt.Errorf("engine: pull request #%d has no commits", pr.Number)
		}

		firstCommit := strings.TrimSpace(pr.Commits[0].OID)
		if firstCommit == "" {
			return ImportPlan{}, fmt.Errorf("engine: pull request #%d contains commit with empty oid", pr.Number)
		}

		parentOut, err := e.repo.Run(ctx, "rev-parse", fmt.Sprintf("%s^", firstCommit))
		if err != nil {
			return ImportPlan{}, fmt.Errorf("engine: git rev-parse %s^: %w", firstCommit, err)
		}
		parentCommit := strings.TrimSpace(parentOut)
		if parentCommit == "" {
			return ImportPlan{}, fmt.Errorf("engine: parent commit for %s is empty", current)
		}

		base := strings.TrimSpace(pr.BaseRef)
		if base == "" {
			return ImportPlan{}, fmt.Errorf("engine: pull request #%d has empty base branch", pr.Number)
		}

		// Ensure the parent branch exists locally before proceeding.
		baseRef := fmt.Sprintf("refs/heads/%s", base)
		baseCommit, err := e.repo.ReadRef(ctx, baseRef)
		if err != nil {
			return ImportPlan{}, err
		}
		if strings.TrimSpace(baseCommit) == "" {
			return ImportPlan{}, fmt.Errorf("engine: branch %s not found", base)
		}

		plan.Lookups = append(plan.Lookups, current)
		plan.Actions = append(plan.Actions, ImportAction{Branch: current, Parent: base, ParentCommit: parentCommit})

		current = base
	}

	if plan.BaseBranch == "" {
		plan.BaseBranch = current
	}

	// actions were recorded top-down; reverse for execution (bottom-up)
	for i, j := 0, len(plan.Actions)-1; i < j; i, j = i+1, j-1 {
		plan.Actions[i], plan.Actions[j] = plan.Actions[j], plan.Actions[i]
	}

	return plan, nil
}

// ExecuteImportPlan applies the parent relationships described by plan to the repository.
func (e *Engine) ExecuteImportPlan(ctx context.Context, plan ImportPlan) error {
	for _, action := range plan.Actions {
		branch := strings.TrimSpace(action.Branch)
		parent := strings.TrimSpace(action.Parent)
		commit := strings.TrimSpace(action.ParentCommit)

		if branch == "" {
			return fmt.Errorf("engine: import action missing branch name")
		}
		if parent == "" {
			return fmt.Errorf("engine: import action for %s missing parent branch", branch)
		}
		if commit == "" {
			return fmt.Errorf("engine: import action for %s has empty parent commit", branch)
		}

		if _, err := e.repo.Run(ctx, "config", fmt.Sprintf("branch.%s.remote", branch), "."); err != nil {
			return fmt.Errorf("engine: git config branch.%s.remote: %w", branch, err)
		}
		if _, err := e.repo.Run(ctx, "config", fmt.Sprintf("branch.%s.merge", branch), fmt.Sprintf("refs/heads/%s", parent)); err != nil {
			return fmt.Errorf("engine: git config branch.%s.merge: %w", branch, err)
		}
		if err := e.repo.WriteRef(ctx, fmt.Sprintf("refs/stack-parent/%s", branch), commit, ""); err != nil {
			return fmt.Errorf("engine: update parent ref for %s: %w", branch, err)
		}
	}

	return nil
}
