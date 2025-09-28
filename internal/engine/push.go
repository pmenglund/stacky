package engine

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"sort"
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
	Remote    string
	Actions   []PushAction
	IncludePR bool
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

	plan := PushPlan{Remote: remote, IncludePR: includePR}

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

	if plan.IncludePR {
		if err := e.updateStackComments(ctx, plan); err != nil {
			return err
		}
	}

	return nil
}

const (
	stackCommentStart = "<!-- Stacky Stack Info -->"
	stackCommentEnd   = "<!-- End Stacky Stack Info -->"
)

func (e *Engine) updateStackComments(ctx context.Context, plan PushPlan) error {
	branchSet := make(map[string]struct{})
	for _, action := range plan.Actions {
		if action.IsBase {
			continue
		}
		branchSet[action.Branch] = struct{}{}
	}
	if len(branchSet) == 0 {
		return nil
	}

	prs, err := e.OpenPullRequests(ctx)
	if err != nil {
		return err
	}

	filtered := make(map[string]githubcli.PullRequest)
	for name := range branchSet {
		if pr, ok := prs[name]; ok {
			filtered[name] = pr
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	graph, err := e.loadGraph(ctx)
	if err != nil {
		return err
	}

	bottoms, err := e.stackBottomNames(ctx)
	if err != nil {
		return err
	}

	type rootInfo struct {
		root    *stackgraph.Branch
		targets map[string]struct{}
	}

	roots := make(map[string]*rootInfo)
	for name := range filtered {
		branch, ok := graph.Branch(name)
		if !ok {
			continue
		}
		root := branch
		for parent := branch.Parent; parent != nil; parent = parent.Parent {
			if _, bottom := bottoms[parent.Name]; bottom {
				break
			}
			root = parent
		}
		info := roots[root.Name]
		if info == nil {
			info = &rootInfo{root: root, targets: make(map[string]struct{})}
			roots[root.Name] = info
		}
		info.targets[branch.Name] = struct{}{}
	}

	if len(roots) == 0 {
		return nil
	}

	for _, info := range roots {
		for target := range info.targets {
			pr := filtered[target]
			comment := renderStackComment(info.root, target, prs, bottoms)
			if strings.TrimSpace(comment) == "" {
				continue
			}
			newBody, changed := applyStackComment(pr.Body, comment)
			if !changed {
				continue
			}
			if err := e.gh.EditPRBody(ctx, pr.Number, newBody); err != nil {
				return err
			}
			pr.Body = newBody
			prs[target] = pr
			filtered[target] = pr
		}
	}

	return nil
}

func renderStackComment(root *stackgraph.Branch, target string, prs map[string]githubcli.PullRequest, bottoms map[string]struct{}) string {
	if root == nil {
		return ""
	}

	lines := make([]string, 0)
	var walk func(*stackgraph.Branch, int)
	walk = func(branch *stackgraph.Branch, depth int) {
		if branch == nil {
			return
		}

		if _, isBottom := bottoms[branch.Name]; !isBottom {
			line := fmt.Sprintf("%s- %s", strings.Repeat("  ", depth), branch.Name)
			if pr, ok := prs[branch.Name]; ok && pr.Number != 0 {
				line += fmt.Sprintf(" (#%d%s)", pr.Number, prStatusEmoji(pr))
			}
			if branch.Name == target {
				line += " ← (CURRENT PR)"
			}
			lines = append(lines, line)
		}

		children := append([]*stackgraph.Branch(nil), branch.Children...)
		sort.Slice(children, func(i, j int) bool {
			return strings.ToLower(children[i].Name) < strings.ToLower(children[j].Name)
		})
		for _, child := range children {
			walk(child, depth+1)
		}
	}

	walk(root, 0)
	if len(lines) == 0 {
		return ""
	}

	out := make([]string, 0, len(lines)+3)
	out = append(out, stackCommentStart)
	out = append(out, "**Stack:**")
	out = append(out, lines...)
	out = append(out, stackCommentEnd)
	return strings.Join(out, "\n")
}

func applyStackComment(body, comment string) (string, bool) {
	if strings.TrimSpace(comment) == "" {
		return body, false
	}

	existing := extractStackComment(body)
	if existing == "" {
		if strings.TrimSpace(body) == "" {
			return comment, true
		}
		return body + "\n\n" + comment, true
	}

	if existing == comment {
		return body, false
	}

	return strings.Replace(body, existing, comment, 1), true
}

func extractStackComment(body string) string {
	start := strings.Index(body, stackCommentStart)
	if start == -1 {
		return ""
	}
	endIdx := strings.Index(body[start:], stackCommentEnd)
	if endIdx == -1 {
		return ""
	}
	end := start + endIdx + len(stackCommentEnd)
	return body[start:end]
}

func prStatusEmoji(pr githubcli.PullRequest) string {
	if pr.IsDraft {
		return " 🚧"
	}
	if strings.EqualFold(pr.ReviewDecision, "APPROVED") {
		return " ✅"
	}
	if len(pr.ReviewRequests) > 0 {
		return " 🔄"
	}
	return " ❌"
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
