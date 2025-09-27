package engine

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/pmenglund/stacky/internal/githubcli"
	"github.com/pmenglund/stacky/internal/stackgraph"
)

// PRAction describes how stacky should manipulate pull requests during a push.
type PRAction int

const (
	// PRActionNone performs no pull request changes.
	PRActionNone PRAction = iota
	// PRActionCreate opens a new pull request for the branch.
	PRActionCreate
	// PRActionUpdateBase retargets an existing pull request to the branch parent.
	PRActionUpdateBase
)

// PushAction captures work to perform for a single branch.
type PushAction struct {
	Branch       string
	Parent       string
	ParentCommit string
	LocalCommit  string
	RemoteCommit string
	RemoteExists bool
	Push         bool
	IsBase       bool

	HasPR    bool
	PRNumber int
	PRBase   string
	PRAction PRAction
}

// PushPlan enumerates the git pushes and pull-request updates required.
type PushPlan struct {
	Remote  string
	Actions []PushAction
}

// PlanStackPush builds a PushPlan for the full stack rooted at the current branch.
func (e *Engine) PlanStackPush(ctx context.Context, remote string, includePR bool) (PushPlan, error) {
	graph, err := e.loadGraph(ctx)
	if err != nil {
		return PushPlan{}, err
	}

	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return PushPlan{}, err
	}

	down, err := graph.Downstack(current)
	if err != nil {
		return PushPlan{}, err
	}
	if len(down) == 0 {
		return PushPlan{}, fmt.Errorf("engine: branch %s has empty stack", current)
	}

	root := down[len(down)-1]
	forest := []*stackgraph.Branch{root}
	return e.planPush(ctx, forest, remote, includePR)
}

// PlanDownstackPush builds a PushPlan for the current branch and its ancestors.
func (e *Engine) PlanDownstackPush(ctx context.Context, remote string, includePR bool) (PushPlan, error) {
	graph, err := e.loadGraph(ctx)
	if err != nil {
		return PushPlan{}, err
	}

	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return PushPlan{}, err
	}

	branch, ok := graph.Branch(current)
	if !ok {
		return PushPlan{}, fmt.Errorf("engine: branch %s not found", current)
	}

	forest := downstackForest(branch)
	if len(forest) == 0 {
		return PushPlan{}, nil
	}
	return e.planPush(ctx, forest, remote, includePR)
}

// PlanUpstackPush builds a PushPlan for the current branch and its descendants.
func (e *Engine) PlanUpstackPush(ctx context.Context, remote string, includePR bool) (PushPlan, error) {
	graph, err := e.loadGraph(ctx)
	if err != nil {
		return PushPlan{}, err
	}

	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return PushPlan{}, err
	}

	branch, ok := graph.Branch(current)
	if !ok {
		return PushPlan{}, fmt.Errorf("engine: branch %s not found", current)
	}

	forest := []*stackgraph.Branch{branch}
	return e.planPush(ctx, forest, remote, includePR)
}

func (e *Engine) planPush(ctx context.Context, forest []*stackgraph.Branch, remote string, includePR bool) (PushPlan, error) {
	if remote == "" {
		remote = "origin"
	}

	plan := PushPlan{Remote: remote}

	branches := gatherBranches(forest)
	if len(branches) == 0 {
		return plan, nil
	}

	var prs map[string]githubcli.PullRequest
	var err error
	if includePR {
		prs, err = e.OpenPullRequests(ctx)
		if err != nil {
			return PushPlan{}, err
		}
	}

	for _, branch := range branches {
		action := PushAction{
			Branch:       branch.Name,
			LocalCommit:  strings.TrimSpace(branch.Commit),
			ParentCommit: strings.TrimSpace(branch.ParentCommit),
		}

		if branch.Parent == nil {
			action.IsBase = true
			plan.Actions = append(plan.Actions, action)
			continue
		}

		action.Parent = branch.Parent.Name

		parentCommit := strings.TrimSpace(branch.Parent.Commit)
		if action.ParentCommit != "" && action.ParentCommit != parentCommit {
			return PushPlan{}, fmt.Errorf("engine: branch %s is not synced with parent %s", branch.Name, branch.Parent.Name)
		}

		remoteRef := fmt.Sprintf("refs/remotes/%s/%s", remote, branch.Name)
		remoteCommit, err := e.repo.ReadRef(ctx, remoteRef)
		if err != nil {
			return PushPlan{}, err
		}

		action.RemoteCommit = strings.TrimSpace(remoteCommit)
		action.RemoteExists = action.RemoteCommit != ""
		action.Push = !action.RemoteExists || action.RemoteCommit != action.LocalCommit

		if includePR {
			if pr, ok := prs[branch.Name]; ok {
				action.HasPR = true
				action.PRNumber = pr.Number
				action.PRBase = pr.BaseRef
				if !strings.EqualFold(pr.BaseRef, branch.Parent.Name) {
					action.PRAction = PRActionUpdateBase
				}
			} else {
				action.PRAction = PRActionCreate
			}
		}

		plan.Actions = append(plan.Actions, action)
	}

	return plan, nil
}

// ExecutePushPlan performs the git and GitHub operations described by plan.
func (e *Engine) ExecutePushPlan(ctx context.Context, plan PushPlan) error {
	remote := plan.Remote
	if remote == "" {
		remote = "origin"
	}

	var prPrefix string
	var prPrefixComputed bool

	for _, action := range plan.Actions {
		if action.IsBase {
			continue
		}

		if action.Push {
			args := []string{"push"}
			if e.cfg.UseForcePush {
				args = append(args, "-f")
			}
			args = append(args, remote, fmt.Sprintf("refs/heads/%s", action.Branch))
			if _, err := e.repo.Run(ctx, args...); err != nil {
				return err
			}
		}

		switch action.PRAction {
		case PRActionNone:
		case PRActionUpdateBase:
			if action.PRNumber == 0 {
				return fmt.Errorf("engine: missing PR number for branch %s", action.Branch)
			}
			if err := e.gh.EditPRBase(ctx, action.PRNumber, action.Parent); err != nil {
				return err
			}
		case PRActionCreate:
			if prPrefixComputed {
				// already computed
			} else {
				var err error
				prPrefix, err = e.computePRPrefix(ctx, remote)
				if err != nil {
					return err
				}
				prPrefixComputed = true
			}
			if action.Parent == "" {
				return fmt.Errorf("engine: cannot create PR for branch %s without parent", action.Branch)
			}
			params := githubcli.CreateParams{Head: prPrefix + action.Branch, Base: action.Parent}
			if err := e.gh.CreatePR(ctx, params); err != nil {
				return err
			}
		}
	}

	return nil
}

func (e *Engine) computePRPrefix(ctx context.Context, remote string) (string, error) {
	resolved, err := e.repo.Config(ctx, fmt.Sprintf("remote.%s.gh-resolved", remote))
	if err != nil {
		return "", err
	}
	if resolved == "" || !strings.Contains(resolved, "/") {
		return "", nil
	}

	remoteURL, err := e.repo.Config(ctx, fmt.Sprintf("remote.%s.url", remote))
	if err != nil {
		return "", err
	}
	owner := extractRepoOwner(remoteURL)
	if owner == "" {
		return "", nil
	}
	return owner + ":", nil
}

func extractRepoOwner(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if strings.HasPrefix(raw, "git@") {
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) == 2 {
			withoutSuffix := strings.TrimSuffix(parts[1], ".git")
			segs := strings.Split(withoutSuffix, "/")
			if len(segs) >= 1 {
				return segs[0]
			}
		}
		return ""
	}

	if strings.HasPrefix(raw, "ssh://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		withoutSuffix := strings.TrimSuffix(u.Path, ".git")
		segs := strings.Split(strings.TrimPrefix(withoutSuffix, "/"), "/")
		if len(segs) >= 1 {
			return segs[0]
		}
		return ""
	}

	// fallback: treat as path-like owner/repo
	trimmed := strings.TrimSuffix(raw, ".git")
	_, owner := path.Split(path.Dir(trimmed))
	return owner
}
