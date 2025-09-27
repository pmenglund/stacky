package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pmenglund/stacky/internal/config"
	"github.com/pmenglund/stacky/internal/githubcli"
	"github.com/pmenglund/stacky/internal/gitstore"
	"github.com/pmenglund/stacky/internal/stackgraph"
	"github.com/pmenglund/stacky/internal/state"
)

// Options controls Engine construction.
type Options struct {
	RepoPath  string
	HomeDir   string
	StatePath string
	GitHub    githubcli.Client
}

// Engine coordinates high level stack operations.
type Engine struct {
	repo      *gitstore.Repository
	cfg       config.Config
	statePath string
	gh        githubcli.Client
}

// PushOptions controls stack push operations.
type PushOptions struct {
	Remote    string
	IncludePR bool
}

// CommitOptions describes how git commits should be created.
type CommitOptions struct {
	Message    string
	AddAll     bool
	AllowEmpty bool
	NoVerify   bool
	Amend      bool
	NoEdit     bool
}

// New wires together the core helpers used by the CLI.
func New(ctx context.Context, opts Options) (*Engine, error) {
	repoPath := opts.RepoPath
	if repoPath == "" {
		repoPath = "."
	}

	repo, err := gitstore.Open(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("engine: open repo: %w", err)
	}

	cfg, err := config.Load(ctx, opts.HomeDir, repo.Root())
	if err != nil {
		return nil, fmt.Errorf("engine: load config: %w", err)
	}

	statePath := opts.StatePath
	if statePath == "" && opts.HomeDir != "" {
		statePath = filepath.Join(opts.HomeDir, ".stacky.state")
	}

	gh := opts.GitHub
	if gh == nil {
		gh = githubcli.New()
	}

	return &Engine{
		repo:      repo,
		cfg:       cfg,
		statePath: statePath,
		gh:        gh,
	}, nil
}

// Config exposes the merged stacky configuration.
func (e *Engine) Config() config.Config {
	return e.cfg
}

// StatePath returns the path used for storing stacky temporary state.
func (e *Engine) StatePath() string {
	return e.statePath
}

// CurrentBranch returns the repository's current branch name.
func (e *Engine) CurrentBranch(ctx context.Context) (string, error) {
	return e.repo.CurrentBranch(ctx)
}

// Checkout switches the working tree to the specified branch.
func (e *Engine) Checkout(ctx context.Context, branch string) error {
	return e.repo.Checkout(ctx, branch)
}

// Branch returns the stackgraph.Branch for the given name.
func (e *Engine) Branch(ctx context.Context, name string) (*stackgraph.Branch, error) {
	graph, err := e.loadGraph(ctx)
	if err != nil {
		return nil, err
	}
	branch, ok := graph.Branch(name)
	if !ok {
		return nil, fmt.Errorf("engine: branch %s not found", name)
	}
	return branch, nil
}

// StackForest loads all stacks as a forest of Branch pointers ready for rendering.
func (e *Engine) StackForest(ctx context.Context) ([]*stackgraph.Branch, error) {
	graph, err := e.loadGraph(ctx)
	if err != nil {
		return nil, err
	}
	return graph.Forest(), nil
}

// gatherBranches returns a deterministic depth-first ordering of the forest.
func gatherBranches(roots []*stackgraph.Branch) []*stackgraph.Branch {
	var result []*stackgraph.Branch
	var walk func(*stackgraph.Branch)
	walk = func(b *stackgraph.Branch) {
		if b == nil {
			return
		}
		result = append(result, b)
		if len(b.Children) == 0 {
			return
		}
		sort.Slice(b.Children, func(i, j int) bool {
			return b.Children[i].Name < b.Children[j].Name
		})
		for _, child := range b.Children {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root)
	}
	return result
}

func (e *Engine) syncForest(ctx context.Context, forest []*stackgraph.Branch, originalBranch string) error {
	branches := gatherBranches(forest)
	if len(branches) == 0 {
		return nil
	}

	// snapshot current commits
	for _, b := range branches {
		head, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", b.Name))
		if err != nil {
			return err
		}
		b.Commit = strings.TrimSpace(head)
	}

	for _, b := range branches {
		if b.Parent == nil {
			continue
		}

		parentHead, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", b.Parent.Name))
		if err != nil {
			return err
		}
		parentHead = strings.TrimSpace(parentHead)
		if parentHead == "" {
			return fmt.Errorf("engine: parent branch %s has no commit", b.Parent.Name)
		}

		if strings.TrimSpace(b.ParentCommit) == parentHead {
			b.Parent.Commit = parentHead
			continue
		}

		if _, err := e.repo.Run(ctx, "checkout", b.Name); err != nil {
			return err
		}

		if e.cfg.UseMerge {
			if _, err := e.repo.Run(ctx, "merge", b.Parent.Name); err != nil {
				_, _ = e.repo.Run(ctx, "merge", "--abort")
				_, _ = e.repo.Run(ctx, "checkout", originalBranch)
				return err
			}
		} else {
			if _, err := e.repo.Run(ctx, "rebase", b.Parent.Name); err != nil {
				_, _ = e.repo.Run(ctx, "rebase", "--abort")
				_, _ = e.repo.Run(ctx, "checkout", originalBranch)
				return err
			}
		}

		if _, err := e.repo.Run(ctx, "checkout", originalBranch); err != nil {
			return err
		}

		newHead, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", b.Name))
		if err != nil {
			return err
		}

		if err := e.repo.WriteRef(ctx, fmt.Sprintf("refs/stack-parent/%s", b.Name), parentHead, b.ParentCommit); err != nil {
			return err
		}
		b.Parent.Commit = parentHead
		b.ParentCommit = parentHead
		b.Commit = strings.TrimSpace(newHead)
	}

	if _, err := e.repo.Run(ctx, "checkout", originalBranch); err != nil {
		return err
	}

	return nil
}

// CreateBranch creates a new branch off the current branch and updates the
// stack-parent ref to mirror legacy behaviour.
func (e *Engine) CreateBranch(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("engine: branch name must not be empty")
	}

	parentBranch, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	parentCommit, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", parentBranch))
	if err != nil {
		return err
	}
	if parentCommit == "" {
		return fmt.Errorf("engine: parent branch %s has no commit", parentBranch)
	}

	if err := e.repo.CreateBranch(ctx, name); err != nil {
		return fmt.Errorf("engine: create branch %s: %w", name, err)
	}

	if err := e.repo.WriteRef(ctx, fmt.Sprintf("refs/stack-parent/%s", name), parentCommit, ""); err != nil {
		return fmt.Errorf("engine: update parent ref for %s: %w", name, err)
	}

	return nil
}

// Commit creates a git commit using the provided options.
func (e *Engine) Commit(ctx context.Context, opts CommitOptions) error {
	graph, err := e.loadGraph(ctx)
	if err != nil {
		return err
	}

	branchName, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	branch, ok := graph.Branch(branchName)
	if !ok {
		return fmt.Errorf("engine: branch %s not found", branchName)
	}

	if branch.Parent == nil {
		return fmt.Errorf("engine: do not commit directly on %s", branch.Name)
	}

	if branch.ParentCommit != branch.Parent.Commit {
		return fmt.Errorf("engine: branch %s is not synced with parent %s", branch.Name, branch.Parent.Name)
	}

	if opts.Amend {
		if e.cfg.UseMerge || !e.cfg.UseForcePush {
			return fmt.Errorf("engine: amending is not allowed when merge commits are used or force push disabled")
		}
		if branch.Commit == branch.Parent.Commit {
			return fmt.Errorf("engine: branch %s has no commits to amend", branch.Name)
		}
	}

	if opts.NoEdit && !opts.Amend {
		return fmt.Errorf("engine: --no-edit only supported with --amend")
	}

	if opts.AddAll {
		if _, err := e.repo.Run(ctx, "add", "--all"); err != nil {
			return fmt.Errorf("engine: git add --all: %w", err)
		}
	}

	args := []string{"commit"}
	if opts.Amend {
		args = append(args, "--amend")
	}
	if opts.NoEdit {
		args = append(args, "--no-edit")
	}
	if opts.AllowEmpty {
		args = append(args, "--allow-empty")
	}
	if opts.NoVerify {
		args = append(args, "--no-verify")
	}
	if opts.Message != "" {
		args = append(args, "-m", opts.Message)
	}

	if _, err := e.repo.Run(ctx, args...); err != nil {
		return fmt.Errorf("engine: git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// Fold merges the current branch into its parent and deletes the branch while
// reparenting any descendants to the parent.
func (e *Engine) Fold(ctx context.Context, allowEmpty bool) error {
	graph, err := e.loadGraph(ctx)
	if err != nil {
		return err
	}

	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	branch, ok := graph.Branch(current)
	if !ok {
		return fmt.Errorf("engine: branch %s not found", current)
	}
	if branch.Parent == nil {
		return fmt.Errorf("engine: cannot fold stack bottom branch %s", branch.Name)
	}
	parent := branch.Parent
	if parent.Parent == nil {
		return fmt.Errorf("engine: cannot fold into stack bottom branch %s", parent.Name)
	}

	parentCommit := strings.TrimSpace(parent.Commit)
	if parentCommit == "" {
		return fmt.Errorf("engine: parent branch %s has no commit", parent.Name)
	}
	trackedParent := strings.TrimSpace(branch.ParentCommit)
	if trackedParent == "" {
		return fmt.Errorf("engine: branch %s has no recorded parent commit", branch.Name)
	}
	if trackedParent != parentCommit {
		return fmt.Errorf("engine: branch %s is not synced with parent %s", branch.Name, parent.Name)
	}

	headCommit := strings.TrimSpace(branch.Commit)
	if headCommit == "" {
		return fmt.Errorf("engine: branch %s has no commit", branch.Name)
	}

	if err := e.repo.Checkout(ctx, parent.Name); err != nil {
		return err
	}

	commits := []string{}
	if parentCommit != headCommit {
		revRange := fmt.Sprintf("%s..%s", parentCommit, headCommit)
		out, err := e.repo.Run(ctx, "rev-list", "--reverse", revRange)
		if err != nil {
			return fmt.Errorf("engine: git rev-list --reverse %s: %w", revRange, err)
		}
		commits = parseRevList(out)
	}

	if e.cfg.UseMerge {
		if _, err := e.repo.Run(ctx, "merge", branch.Name); err != nil {
			return fmt.Errorf("engine: git merge %s: %w", branch.Name, err)
		}
	} else {
		for _, commit := range commits {
			if err := e.applyCherryPick(ctx, commit, allowEmpty); err != nil {
				return err
			}
		}
	}

	parentHead, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", parent.Name))
	if err != nil {
		return err
	}
	parentHead = strings.TrimSpace(parentHead)
	if parentHead == "" {
		return fmt.Errorf("engine: parent branch %s has no head commit", parent.Name)
	}

	for _, child := range branch.Children {
		if _, err := e.repo.Run(ctx, "config", fmt.Sprintf("branch.%s.merge", child.Name), fmt.Sprintf("refs/heads/%s", parent.Name)); err != nil {
			return fmt.Errorf("engine: git config branch.%s.merge: %w", child.Name, err)
		}
		oldParent := strings.TrimSpace(child.ParentCommit)
		if err := e.repo.WriteRef(ctx, fmt.Sprintf("refs/stack-parent/%s", child.Name), parentHead, oldParent); err != nil {
			return fmt.Errorf("engine: update parent ref for %s: %w", child.Name, err)
		}
	}

	if _, err := e.repo.Run(ctx, "branch", "-D", branch.Name); err != nil {
		return fmt.Errorf("engine: git branch -D %s: %w", branch.Name, err)
	}
	if _, err := e.repo.Run(ctx, "update-ref", "-d", fmt.Sprintf("refs/stack-parent/%s", branch.Name)); err != nil {
		return fmt.Errorf("engine: remove stack parent ref for %s: %w", branch.Name, err)
	}

	return nil
}

// Log returns the repository log output honoring configuration options.
func (e *Engine) Log(ctx context.Context) (string, error) {
	args := []string{"log"}
	if e.cfg.UseMerge {
		args = append(args, "--no-merges", "--first-parent")
	}
	out, err := e.repo.Run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("engine: git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// StackSync rebases or merges branches in the current stack so each branch is
// aligned with its parent.
func (e *Engine) StackSync(ctx context.Context) error {
	graph, err := e.loadGraph(ctx)
	if err != nil {
		return err
	}

	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	downstack, err := graph.Downstack(current)
	if err != nil {
		return err
	}
	if len(downstack) == 0 {
		return fmt.Errorf("engine: empty stack for branch %s", current)
	}
	root := downstack[len(downstack)-1]

	return e.syncForest(ctx, []*stackgraph.Branch{root}, current)
}

// Update fetches the remote and fast-forwards stack bottom branches to match
// their remote counterparts. It returns the list of branches updated.
func (e *Engine) Update(ctx context.Context, force bool) ([]string, error) {
	// TODO: respect configured remote and handle SSH muxing when implemented.
	if _, err := e.repo.Run(ctx, "fetch", "origin"); err != nil {
		return nil, fmt.Errorf("engine: git fetch origin: %w", err)
	}

	graph, err := e.loadGraph(ctx)
	if err != nil {
		return nil, err
	}

	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return nil, err
	}

	bottoms := graph.Bottoms()
	updated := make([]string, 0, len(bottoms))
	for _, bottom := range bottoms {
		remoteRef := fmt.Sprintf("refs/remotes/origin/%s", bottom.Name)
		remoteCommit, err := e.repo.ReadRef(ctx, remoteRef)
		if err != nil {
			return nil, fmt.Errorf("engine: read %s: %w", remoteRef, err)
		}
		if remoteCommit == "" {
			return nil, fmt.Errorf("engine: remote branch %s not found", remoteRef)
		}

		localRef := fmt.Sprintf("refs/heads/%s", bottom.Name)
		if err := e.repo.WriteRef(ctx, localRef, remoteCommit, ""); err != nil {
			return nil, fmt.Errorf("engine: update %s: %w", localRef, err)
		}

		if bottom.Name == current {
			if _, err := e.repo.Run(ctx, "reset", "--hard", "HEAD"); err != nil {
				return nil, fmt.Errorf("engine: git reset --hard HEAD: %w", err)
			}
		}

		updated = append(updated, bottom.Name)
	}

	return updated, nil
}

// Continue attempts to resume a paused stack operation using the persisted state.
// Currently only branch checkout resumption is supported.
func (e *Engine) Continue(ctx context.Context) (string, error) {
	if e.statePath == "" {
		return "", fmt.Errorf("engine: state path not configured")
	}

	st, err := state.Load(e.statePath)
	if err != nil {
		return "", err
	}
	if st.Branch == "" && len(st.Sync) == 0 && st.Fold == nil && st.MergeFold == nil {
		return "", fmt.Errorf("engine: no pending operation to continue")
	}

	if len(st.Sync) > 0 || st.Fold != nil || st.MergeFold != nil {
		return "", fmt.Errorf("engine: continue for sync/fold operations not implemented")
	}

	if st.Branch == "" {
		return "", fmt.Errorf("engine: stored branch state missing branch name")
	}

	if _, err := e.Branch(ctx, st.Branch); err != nil {
		return "", err
	}

	if err := e.Checkout(ctx, st.Branch); err != nil {
		return "", err
	}

	if err := state.Clear(e.statePath); err != nil {
		return "", err
	}

	return st.Branch, nil
}

// CurrentStackRoot returns the root branch for the current stack.
func (e *Engine) CurrentStackRoot(ctx context.Context) (*stackgraph.Branch, error) {
	graph, err := e.loadGraph(ctx)
	if err != nil {
		return nil, err
	}
	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return nil, err
	}

	down, err := graph.Downstack(current)
	if err != nil {
		return nil, err
	}
	if len(down) == 0 {
		return nil, fmt.Errorf("engine: branch %s has empty stack", current)
	}
	root := down[len(down)-1]
	return root, nil
}

func (e *Engine) loadGraph(ctx context.Context) (*stackgraph.Graph, error) {
	graph, err := stackgraph.Load(ctx, e.repo)
	if err != nil {
		return nil, fmt.Errorf("engine: load stack graph: %w", err)
	}
	return graph, nil
}

func (e *Engine) StackPush(ctx context.Context, opts PushOptions) error {
	plan, err := e.PlanStackPush(ctx, opts.Remote, opts.IncludePR)
	if err != nil {
		return err
	}
	return e.ExecutePushPlan(ctx, plan)
}

// DownstackSync rebases or merges the current branch and its ancestors.
func (e *Engine) DownstackSync(ctx context.Context) error {
	graph, err := e.loadGraph(ctx)
	if err != nil {
		return err
	}

	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	branch, ok := graph.Branch(current)
	if !ok {
		return fmt.Errorf("engine: branch %s not found", current)
	}

	forest := downstackForest(branch)
	if len(forest) == 0 {
		return nil
	}

	return e.syncForest(ctx, forest, current)
}

// DownstackPush pushes the current branch and its ancestors to the remote.
func (e *Engine) DownstackPush(ctx context.Context, opts PushOptions) error {
	plan, err := e.PlanDownstackPush(ctx, opts.Remote, opts.IncludePR)
	if err != nil {
		return err
	}
	return e.ExecutePushPlan(ctx, plan)
}

// UpstackSync syncs the current branch and its descendants.
func (e *Engine) UpstackSync(ctx context.Context) error {
	graph, err := e.loadGraph(ctx)
	if err != nil {
		return err
	}

	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	branch, ok := graph.Branch(current)
	if !ok {
		return fmt.Errorf("engine: branch %s not found", current)
	}

	return e.syncForest(ctx, []*stackgraph.Branch{branch}, current)
}

// UpstackPush pushes the current branch and descendants to the remote.
func (e *Engine) UpstackPush(ctx context.Context, opts PushOptions) error {
	plan, err := e.PlanUpstackPush(ctx, opts.Remote, opts.IncludePR)
	if err != nil {
		return err
	}
	return e.ExecutePushPlan(ctx, plan)
}

// OpenPullRequests returns open PRs keyed by head branch name.
func (e *Engine) OpenPullRequests(ctx context.Context) (map[string]githubcli.PullRequest, error) {
	prs, err := e.gh.ListPRs(ctx, githubcli.ListParams{State: "open"})
	if err != nil {
		return nil, err
	}
	result := make(map[string]githubcli.PullRequest, len(prs))
	for _, pr := range prs {
		if pr.HeadRef == "" {
			continue
		}
		result[pr.HeadRef] = pr
	}
	return result, nil
}

func (e *Engine) AuthoredPullRequests(ctx context.Context) ([]githubcli.PullRequest, error) {
	return e.gh.ListPRs(ctx, githubcli.ListParams{State: "open", Author: "@me"})
}

func (e *Engine) ReviewRequestedPullRequests(ctx context.Context) ([]githubcli.PullRequest, error) {
	return e.gh.ListPRs(ctx, githubcli.ListParams{State: "open", Search: "review-requested:@me"})
}

func cloneBranchShallow(src *stackgraph.Branch) *stackgraph.Branch {
	if src == nil {
		return nil
	}
	return &stackgraph.Branch{
		Name:         src.Name,
		ParentCommit: src.ParentCommit,
		Commit:       src.Commit,
	}
}

func (e *Engine) applyCherryPick(ctx context.Context, commit string, allowEmpty bool) error {
	commit = strings.TrimSpace(commit)
	if commit == "" {
		return nil
	}

	if allowEmpty {
		if _, err := e.repo.Run(ctx, "cherry-pick", "--allow-empty", commit); err != nil {
			return fmt.Errorf("engine: git cherry-pick --allow-empty %s: %w", commit, err)
		}
		return nil
	}

	if _, err := e.repo.Run(ctx, "cherry-pick", "--no-commit", commit); err != nil {
		_, _ = e.repo.Run(ctx, "reset", "--hard", "HEAD")
		if _, err := e.repo.Run(ctx, "cherry-pick", commit); err != nil {
			return fmt.Errorf("engine: git cherry-pick %s: %w", commit, err)
		}
		return nil
	}

	diff, err := e.repo.Run(ctx, "diff", "--cached", "--name-only")
	if err != nil {
		_, _ = e.repo.Run(ctx, "reset", "--hard", "HEAD")
		return fmt.Errorf("engine: git diff --cached --name-only: %w", err)
	}
	_, _ = e.repo.Run(ctx, "reset", "--hard", "HEAD")

	if strings.TrimSpace(diff) == "" {
		return nil
	}

	if _, err := e.repo.Run(ctx, "cherry-pick", commit); err != nil {
		return fmt.Errorf("engine: git cherry-pick %s: %w", commit, err)
	}
	return nil
}

func parseRevList(output string) []string {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	lines := strings.Split(output, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func downstackForest(branch *stackgraph.Branch) []*stackgraph.Branch {
	if branch == nil {
		return nil
	}

	var head *stackgraph.Branch
	for b := branch; b != nil; b = b.Parent {
		clone := cloneBranchShallow(b)
		if head != nil {
			clone.Children = []*stackgraph.Branch{head}
			head.Parent = clone
		}
		head = clone
	}

	if head == nil {
		return nil
	}

	return []*stackgraph.Branch{head}
}
