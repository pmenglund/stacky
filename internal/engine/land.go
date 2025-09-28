package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/pmenglund/stacky/internal/githubcli"
)

// LandPlan captures the information needed to land the bottom-most branch in a stack.
type LandPlan struct {
	Branch        string
	Parent        string
	PRNumber      int
	PRURL         string
	HeadCommit    string
	CurrentBranch string
	BranchCount   int
}

// PlanLand validates the current stack state and returns the plan for landing the
// bottom-most branch above the stack root.
func (e *Engine) PlanLand(ctx context.Context) (LandPlan, error) {
	graph, err := e.loadGraph(ctx)
	if err != nil {
		return LandPlan{}, err
	}

	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return LandPlan{}, err
	}

	downstack, err := graph.Downstack(current)
	if err != nil {
		return LandPlan{}, err
	}
	if len(downstack) < 2 {
		bottom := downstack[len(downstack)-1]
		return LandPlan{}, fmt.Errorf("engine: cannot land stack bottom %s", bottom.Name)
	}

	target := downstack[len(downstack)-2]
	parent := target.Parent
	if parent == nil {
		return LandPlan{}, fmt.Errorf("engine: branch %s has no parent to land onto", target.Name)
	}

	parentCommit := strings.TrimSpace(parent.Commit)
	if parentCommit == "" {
		return LandPlan{}, fmt.Errorf("engine: parent branch %s has no commit", parent.Name)
	}

	trackedParent := strings.TrimSpace(target.ParentCommit)
	if trackedParent == "" {
		return LandPlan{}, fmt.Errorf("engine: branch %s has no recorded parent commit", target.Name)
	}
	if trackedParent != parentCommit {
		return LandPlan{}, fmt.Errorf("engine: branch %s is not synced with parent %s", target.Name, parent.Name)
	}

	headCommit := strings.TrimSpace(target.Commit)
	if headCommit == "" {
		return LandPlan{}, fmt.Errorf("engine: branch %s has no commit", target.Name)
	}

	remoteRef := fmt.Sprintf("refs/remotes/origin/%s", target.Name)
	remoteCommit, err := e.repo.ReadRef(ctx, remoteRef)
	if err != nil {
		return LandPlan{}, err
	}
	remoteCommit = strings.TrimSpace(remoteCommit)
	if remoteCommit == "" {
		return LandPlan{}, fmt.Errorf("engine: branch %s has not been pushed to origin", target.Name)
	}
	if remoteCommit != headCommit {
		return LandPlan{}, fmt.Errorf("engine: branch %s is not synced with remote branch, push local changes before landing", target.Name)
	}

	prs, err := e.OpenPullRequests(ctx)
	if err != nil {
		return LandPlan{}, err
	}
	pr, ok := prs[target.Name]
	if !ok {
		return LandPlan{}, fmt.Errorf("engine: branch %s does not have an open PR", target.Name)
	}
	if !strings.EqualFold(pr.Mergeable, "MERGEABLE") {
		return LandPlan{}, fmt.Errorf("engine: PR #%d for branch %s is not mergeable: %s", pr.Number, target.Name, pr.Mergeable)
	}

	return LandPlan{
		Branch:        target.Name,
		Parent:        parent.Name,
		PRNumber:      pr.Number,
		PRURL:         pr.URL,
		HeadCommit:    headCommit,
		CurrentBranch: current,
		BranchCount:   len(downstack) - 1,
	}, nil
}

// ExecuteLand merges the pull request associated with the provided plan.
func (e *Engine) ExecuteLand(ctx context.Context, plan LandPlan, auto bool) error {
	if plan.Branch == "" {
		return fmt.Errorf("engine: land plan missing branch")
	}
	if plan.HeadCommit == "" {
		return fmt.Errorf("engine: land plan missing head commit")
	}

	params := githubcli.MergeParams{Head: plan.Branch, MatchHead: plan.HeadCommit, Auto: auto}
	return e.gh.MergePR(ctx, params)
}
