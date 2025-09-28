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

var (
	defaultStackBottoms = []string{"main", "master"}
	frozenStackBottoms  = map[string]struct{}{
		"main":   {},
		"master": {},
	}
)

const (
	stackyBottomRefPrefix   = "refs/stacky-bottom-branch/"
	stackyBottomShortPrefix = "stacky-bottom-branch/"
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

// UpdateBottom describes how a stack bottom should be updated during an
// update operation.
type UpdateBottom struct {
	Branch       string
	LocalCommit  string
	RemoteCommit string
	NeedsUpdate  bool
	IsCurrent    bool
}

// UpdateDeletion captures a branch scheduled for deletion because its pull
// request has been merged, along with any children that will be reparented.
type UpdateDeletion struct {
	Branch   string
	Parent   string
	PRNumber int
	Children []string
}

// UpdatePlan enumerates the actions performed by a stack update.
type UpdatePlan struct {
	Remote    string
	Bottoms   []UpdateBottom
	Deletions []UpdateDeletion
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

func (e *Engine) saveState(st state.State) error {
	if e.statePath == "" {
		return nil
	}
	return state.Save(e.statePath, st)
}

func (e *Engine) clearState() error {
	if e.statePath == "" {
		return nil
	}
	return state.Clear(e.statePath)
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

func (e *Engine) planSyncBranches(ctx context.Context, forest []*stackgraph.Branch) ([]*stackgraph.Branch, error) {
	branches := gatherBranches(forest)
	if len(branches) == 0 {
		return nil, nil
	}

	for _, branch := range branches {
		head, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", branch.Name))
		if err != nil {
			return nil, err
		}
		branch.Commit = strings.TrimSpace(head)
	}

	syncSet := make(map[string]struct{})
	var toSync []*stackgraph.Branch

	for _, branch := range branches {
		if branch.Parent == nil {
			continue
		}

		parentHead, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", branch.Parent.Name))
		if err != nil {
			return nil, err
		}
		parentHead = strings.TrimSpace(parentHead)
		if parentHead == "" {
			return nil, fmt.Errorf("engine: parent branch %s has no commit", branch.Parent.Name)
		}
		branch.Parent.Commit = parentHead

		_, parentScheduled := syncSet[branch.Parent.Name]
		if !parentScheduled && strings.TrimSpace(branch.ParentCommit) == parentHead {
			continue
		}

		toSync = append(toSync, branch)
		syncSet[branch.Name] = struct{}{}
	}

	if len(toSync) == 0 {
		return nil, nil
	}

	for i, j := 0, len(toSync)-1; i < j; i, j = i+1, j-1 {
		toSync[i], toSync[j] = toSync[j], toSync[i]
	}

	return toSync, nil
}

func (e *Engine) runSync(ctx context.Context, originalBranch string, stack []*stackgraph.Branch) error {
	if len(stack) == 0 {
		return e.clearState()
	}

	if _, err := e.repo.Run(ctx, "checkout", originalBranch); err != nil {
		return fmt.Errorf("engine: checkout %s: %w", originalBranch, err)
	}

	syncNames := make([]string, len(stack))
	for i, branch := range stack {
		syncNames[i] = branch.Name
	}

	for len(stack) > 0 {
		if err := e.saveState(state.State{Branch: originalBranch, Sync: append([]string(nil), syncNames...)}); err != nil {
			return err
		}

		branch := stack[len(stack)-1]

		parentHead, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", branch.Parent.Name))
		if err != nil {
			return err
		}
		parentHead = strings.TrimSpace(parentHead)
		if parentHead == "" {
			return fmt.Errorf("engine: parent branch %s has no commit", branch.Parent.Name)
		}

		if _, err := e.repo.Run(ctx, "checkout", branch.Name); err != nil {
			return fmt.Errorf("engine: checkout %s: %w", branch.Name, err)
		}

		syncVerb := "rebase"
		var cmdErr error
		if e.cfg.UseMerge {
			syncVerb = "merge"
			_, cmdErr = e.repo.Run(ctx, "merge", branch.Parent.Name)
		} else {
			oldParent := strings.TrimSpace(branch.ParentCommit)
			if oldParent == "" {
				_, cmdErr = e.repo.Run(ctx, "rebase", branch.Parent.Name)
			} else {
				_, cmdErr = e.repo.Run(ctx, "rebase", "--onto", branch.Parent.Name, oldParent, branch.Name)
			}
		}

		if cmdErr != nil {
			return fmt.Errorf(
				"engine: %s branch %s onto %s: %w. Resolve conflicts and run `git %s --continue`, then `stacky continue`",
				syncVerb,
				branch.Name,
				branch.Parent.Name,
				cmdErr,
				syncVerb,
			)
		}

		if _, err := e.repo.Run(ctx, "checkout", originalBranch); err != nil {
			return fmt.Errorf("engine: checkout %s: %w", originalBranch, err)
		}

		newHead, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", branch.Name))
		if err != nil {
			return err
		}
		newHead = strings.TrimSpace(newHead)

		if err := e.repo.WriteRef(ctx, fmt.Sprintf("refs/stack-parent/%s", branch.Name), parentHead, branch.ParentCommit); err != nil {
			return err
		}
		branch.Parent.Commit = parentHead
		branch.ParentCommit = parentHead
		branch.Commit = newHead

		stack = stack[:len(stack)-1]
		syncNames = syncNames[:len(syncNames)-1]
	}

	if err := e.clearState(); err != nil {
		return err
	}

	return nil
}

func (e *Engine) syncForest(ctx context.Context, forest []*stackgraph.Branch, originalBranch string) error {
	stack, err := e.planSyncBranches(ctx, forest)
	if err != nil {
		return err
	}
	if len(stack) == 0 {
		return nil
	}
	return e.runSync(ctx, originalBranch, stack)
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

// Adopt wires an existing branch into the stack rooted at the current branch.
func (e *Engine) Adopt(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("engine: branch name must not be empty")
	}

	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	if name == current {
		return fmt.Errorf("engine: a branch cannot adopt itself")
	}

	bottoms, err := e.stackBottomNames(ctx)
	if err != nil {
		return err
	}

	if _, ok := bottoms[current]; !ok {
		if e.cfg.ChangeToMain {
			realBottom, err := e.realStackBottom(ctx, bottoms)
			if err != nil {
				return err
			}
			if realBottom != "" && realBottom != current {
				if err := e.repo.Checkout(ctx, realBottom); err != nil {
					return fmt.Errorf("engine: checkout %s: %w", realBottom, err)
				}
				current = realBottom
			}
		}
		if _, ok := bottoms[current]; !ok {
			return fmt.Errorf("engine: current branch %s must be a valid stack bottom: %s", current, strings.Join(sortedKeys(bottoms), ", "))
		}
	}

	graph, err := e.loadGraph(ctx)
	if err != nil {
		return err
	}

	currentBranch, ok := graph.Branch(current)
	if !ok {
		return fmt.Errorf("engine: branch %s not found", current)
	}
	if currentBranch.Parent != nil {
		return fmt.Errorf("engine: current branch %s is not a stack bottom", current)
	}

	target, ok := graph.Branch(name)
	if !ok {
		return fmt.Errorf("engine: branch %s not found", name)
	}

	if _, ok := bottoms[name]; ok {
		if _, frozen := frozenStackBottoms[name]; frozen {
			return fmt.Errorf("engine: cannot adopt frozen stack bottoms %v", sortedKeys(frozenStackBottoms))
		}
		ref := stackyBottomRefPrefix + name
		commit, err := e.repo.ReadRef(ctx, ref)
		if err != nil {
			return err
		}
		if strings.TrimSpace(commit) != "" {
			if _, err := e.repo.Run(ctx, "update-ref", "-d", ref); err != nil {
				return fmt.Errorf("engine: git update-ref -d %s: %w", ref, err)
			}
		}
	}

	mergeBaseOut, err := e.repo.Run(ctx, "merge-base", current, name)
	if err != nil {
		return fmt.Errorf("engine: git merge-base %s %s: %w", current, name, err)
	}
	mergeBase := strings.TrimSpace(mergeBaseOut)
	if mergeBase == "" {
		return fmt.Errorf("engine: merge-base for %s and %s is empty", current, name)
	}

	if _, err := e.repo.Run(ctx, "config", fmt.Sprintf("branch.%s.remote", name), "."); err != nil {
		return fmt.Errorf("engine: git config branch.%s.remote: %w", name, err)
	}
	if _, err := e.repo.Run(ctx, "config", fmt.Sprintf("branch.%s.merge", name), fmt.Sprintf("refs/heads/%s", current)); err != nil {
		return fmt.Errorf("engine: git config branch.%s.merge: %w", name, err)
	}
	if err := e.repo.WriteRef(ctx, fmt.Sprintf("refs/stack-parent/%s", name), mergeBase, strings.TrimSpace(target.ParentCommit)); err != nil {
		return fmt.Errorf("engine: update parent ref for %s: %w", name, err)
	}

	if e.cfg.ChangeToAdopted {
		if err := e.repo.Checkout(ctx, name); err != nil {
			return fmt.Errorf("engine: checkout %s: %w", name, err)
		}
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

	childNames := make([]string, len(branch.Children))
	for i, child := range branch.Children {
		childNames[i] = child.Name
	}
	sort.Strings(childNames)

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

	if e.cfg.UseMerge {
		if err := e.saveState(state.State{
			Branch: parent.Name,
			MergeFold: &state.MergeFoldState{
				FoldBranch:   branch.Name,
				ParentBranch: parent.Name,
				Children:     append([]string(nil), childNames...),
			},
		}); err != nil {
			return err
		}
		if _, err := e.repo.Run(ctx, "merge", branch.Name); err != nil {
			return fmt.Errorf("engine: git merge %s: %w. Resolve conflicts and run `git merge --continue`, then `stacky continue`", branch.Name, err)
		}
	} else {
		commits := []string{}
		if parentCommit != headCommit {
			revRange := fmt.Sprintf("%s..%s", parentCommit, headCommit)
			out, err := e.repo.Run(ctx, "rev-list", "--reverse", revRange)
			if err != nil {
				return fmt.Errorf("engine: git rev-list --reverse %s: %w", revRange, err)
			}
			commits = parseRevList(out)
		}
		remaining := append([]string(nil), commits...)
		for len(remaining) > 0 {
			if err := e.saveState(state.State{
				Branch: parent.Name,
				Fold: &state.FoldState{
					FoldBranch:   branch.Name,
					ParentBranch: parent.Name,
					Commits:      append([]string(nil), remaining...),
					Children:     append([]string(nil), childNames...),
					AllowEmpty:   allowEmpty,
				},
			}); err != nil {
				return err
			}

			idx := len(remaining) - 1
			commit := remaining[idx]
			remaining = remaining[:idx]
			if err := e.applyCherryPick(ctx, commit, allowEmpty); err != nil {
				return fmt.Errorf("%w. Resolve conflicts and run `git cherry-pick --continue`, then `stacky continue`", err)
			}
		}
	}

	if err := e.finishFold(ctx, branch.Name, parent.Name, childNames); err != nil {
		return err
	}
	if err := e.clearState(); err != nil {
		return err
	}

	return nil
}

func (e *Engine) finishFold(ctx context.Context, foldBranch, parentBranch string, childNames []string) error {
	parentHead, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", parentBranch))
	if err != nil {
		return err
	}
	parentHead = strings.TrimSpace(parentHead)
	if parentHead == "" {
		return fmt.Errorf("engine: parent branch %s has no head commit", parentBranch)
	}

	for _, child := range childNames {
		if child == "" {
			continue
		}
		if _, err := e.repo.Run(ctx, "config", fmt.Sprintf("branch.%s.merge", child), fmt.Sprintf("refs/heads/%s", parentBranch)); err != nil {
			return fmt.Errorf("engine: git config branch.%s.merge: %w", child, err)
		}
		oldParent, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/stack-parent/%s", child))
		if err != nil {
			return err
		}
		oldParent = strings.TrimSpace(oldParent)
		if err := e.repo.WriteRef(ctx, fmt.Sprintf("refs/stack-parent/%s", child), parentHead, oldParent); err != nil {
			return fmt.Errorf("engine: update parent ref for %s: %w", child, err)
		}
	}

	branchHead, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", foldBranch))
	if err != nil {
		return err
	}
	if strings.TrimSpace(branchHead) != "" {
		if _, err := e.repo.Run(ctx, "branch", "-D", foldBranch); err != nil {
			return fmt.Errorf("engine: git branch -D %s: %w", foldBranch, err)
		}
	}
	if _, err := e.repo.Run(ctx, "update-ref", "-d", fmt.Sprintf("refs/stack-parent/%s", foldBranch)); err != nil {
		return fmt.Errorf("engine: remove stack parent ref for %s: %w", foldBranch, err)
	}

	return nil
}

func (e *Engine) resumeCherryPickFold(ctx context.Context, branch string, fold *state.FoldState) error {
	if fold == nil {
		return fmt.Errorf("engine: missing fold state")
	}

	remaining := append([]string(nil), fold.Commits...)
	childNames := append([]string(nil), fold.Children...)

	for len(remaining) > 0 {
		if err := e.saveState(state.State{
			Branch: branch,
			Fold: &state.FoldState{
				FoldBranch:   fold.FoldBranch,
				ParentBranch: fold.ParentBranch,
				Commits:      append([]string(nil), remaining...),
				Children:     append([]string(nil), childNames...),
				AllowEmpty:   fold.AllowEmpty,
			},
		}); err != nil {
			return err
		}

		idx := len(remaining) - 1
		commit := remaining[idx]
		remaining = remaining[:idx]
		if err := e.applyCherryPick(ctx, commit, fold.AllowEmpty); err != nil {
			return fmt.Errorf("%w. Resolve conflicts and run `git cherry-pick --continue`, then `stacky continue`", err)
		}
	}

	if err := e.finishFold(ctx, fold.FoldBranch, fold.ParentBranch, childNames); err != nil {
		return err
	}

	return e.clearState()
}

func (e *Engine) resumeMergeFold(ctx context.Context, merge *state.MergeFoldState) error {
	if merge == nil {
		return fmt.Errorf("engine: missing merge fold state")
	}

	childNames := append([]string(nil), merge.Children...)
	if err := e.finishFold(ctx, merge.FoldBranch, merge.ParentBranch, childNames); err != nil {
		return err
	}

	return e.clearState()
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

// PlanUpdate fetches the remote and produces an UpdatePlan describing the
// bottom branch fast-forwards and merged branch deletions that should be
// applied.
func (e *Engine) PlanUpdate(ctx context.Context, remote string) (UpdatePlan, error) {
	if remote == "" {
		remote = "origin"
	}

	if _, err := e.repo.Run(ctx, "fetch", remote); err != nil {
		return UpdatePlan{}, fmt.Errorf("engine: git fetch %s: %w", remote, err)
	}

	graph, err := e.loadGraph(ctx)
	if err != nil {
		return UpdatePlan{}, err
	}

	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return UpdatePlan{}, err
	}

	plan := UpdatePlan{Remote: remote}
	bottoms := graph.Bottoms()
	sort.Slice(bottoms, func(i, j int) bool {
		return bottoms[i].Name < bottoms[j].Name
	})
	for _, bottom := range bottoms {
		remoteRef := fmt.Sprintf("refs/remotes/%s/%s", remote, bottom.Name)
		remoteCommit, err := e.repo.ReadRef(ctx, remoteRef)
		if err != nil {
			return UpdatePlan{}, fmt.Errorf("engine: read %s: %w", remoteRef, err)
		}
		remoteCommit = strings.TrimSpace(remoteCommit)
		if remoteCommit == "" {
			return UpdatePlan{}, fmt.Errorf("engine: remote branch %s not found", remoteRef)
		}

		localCommit, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", bottom.Name))
		if err != nil {
			return UpdatePlan{}, fmt.Errorf("engine: read local branch %s: %w", bottom.Name, err)
		}
		localCommit = strings.TrimSpace(localCommit)

		plan.Bottoms = append(plan.Bottoms, UpdateBottom{
			Branch:       bottom.Name,
			LocalCommit:  localCommit,
			RemoteCommit: remoteCommit,
			NeedsUpdate:  remoteCommit != localCommit,
			IsCurrent:    bottom.Name == current,
		})
	}

	deletions, err := e.planMergedDeletions(ctx, graph.Forest())
	if err != nil {
		return UpdatePlan{}, err
	}
	plan.Deletions = deletions

	return plan, nil
}

// ExecuteUpdatePlan performs the git operations described by plan.
func (e *Engine) ExecuteUpdatePlan(ctx context.Context, plan UpdatePlan) error {
	for _, bottom := range plan.Bottoms {
		remoteCommit := strings.TrimSpace(bottom.RemoteCommit)
		if remoteCommit == "" {
			continue
		}
		localRef := fmt.Sprintf("refs/heads/%s", bottom.Branch)
		oldCommit := strings.TrimSpace(bottom.LocalCommit)
		if err := e.repo.WriteRef(ctx, localRef, remoteCommit, oldCommit); err != nil {
			return fmt.Errorf("engine: update %s: %w", localRef, err)
		}
		if bottom.IsCurrent {
			if _, err := e.repo.Run(ctx, "reset", "--hard", "HEAD"); err != nil {
				return fmt.Errorf("engine: git reset --hard HEAD: %w", err)
			}
		}
	}

	for _, deletion := range plan.Deletions {
		if err := e.reparentChildren(ctx, deletion); err != nil {
			return err
		}
		if err := e.deleteBranch(ctx, deletion.Branch, deletion.Parent); err != nil {
			return err
		}
	}

	if err := e.cleanupStackRefs(ctx); err != nil {
		return err
	}

	return nil
}

func (e *Engine) planMergedDeletions(ctx context.Context, forest []*stackgraph.Branch) ([]UpdateDeletion, error) {
	branches := gatherBranches(forest)
	deletions := make([]UpdateDeletion, 0)
	for _, branch := range branches {
		if branch.Parent == nil {
			continue
		}
		prs, err := e.gh.ListPRs(ctx, githubcli.ListParams{Head: branch.Name, State: "all"})
		if err != nil {
			return nil, err
		}

		var merged *githubcli.PullRequest
		hasOpen := false
		for i := range prs {
			state := strings.ToUpper(strings.TrimSpace(prs[i].State))
			switch state {
			case "OPEN":
				hasOpen = true
			case "MERGED":
				if merged == nil {
					merged = &prs[i]
				}
			}
		}
		if hasOpen || merged == nil {
			continue
		}

		children := make([]string, len(branch.Children))
		for i, child := range branch.Children {
			children[i] = child.Name
		}
		sort.Strings(children)

		deletions = append(deletions, UpdateDeletion{
			Branch:   branch.Name,
			Parent:   branch.Parent.Name,
			PRNumber: merged.Number,
			Children: children,
		})
	}
	return deletions, nil
}

func (e *Engine) reparentChildren(ctx context.Context, deletion UpdateDeletion) error {
	if len(deletion.Children) == 0 {
		return nil
	}

	parentHead, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", deletion.Parent))
	if err != nil {
		return fmt.Errorf("engine: read parent branch %s: %w", deletion.Parent, err)
	}
	parentHead = strings.TrimSpace(parentHead)
	if parentHead == "" {
		return fmt.Errorf("engine: parent branch %s has no head commit", deletion.Parent)
	}

	for _, child := range deletion.Children {
		if _, err := e.repo.Run(ctx, "config", fmt.Sprintf("branch.%s.remote", child), "."); err != nil {
			return fmt.Errorf("engine: git config branch.%s.remote: %w", child, err)
		}
		if _, err := e.repo.Run(ctx, "config", fmt.Sprintf("branch.%s.merge", child), fmt.Sprintf("refs/heads/%s", deletion.Parent)); err != nil {
			return fmt.Errorf("engine: git config branch.%s.merge: %w", child, err)
		}

		oldParentCommit, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/stack-parent/%s", child))
		if err != nil {
			return fmt.Errorf("engine: read stack parent for %s: %w", child, err)
		}
		oldParentCommit = strings.TrimSpace(oldParentCommit)
		if err := e.repo.WriteRef(ctx, fmt.Sprintf("refs/stack-parent/%s", child), parentHead, oldParentCommit); err != nil {
			return fmt.Errorf("engine: update parent ref for %s: %w", child, err)
		}
	}

	return nil
}

func (e *Engine) deleteBranch(ctx context.Context, branch, parent string) error {
	current, err := e.repo.CurrentBranch(ctx)
	if err != nil {
		return err
	}
	if branch == current {
		if parent == "" {
			return fmt.Errorf("engine: cannot delete current stack bottom branch %s", branch)
		}
		if err := e.repo.Checkout(ctx, parent); err != nil {
			return fmt.Errorf("engine: checkout %s: %w", parent, err)
		}
	}
	if _, err := e.repo.Run(ctx, "branch", "-D", branch); err != nil {
		return fmt.Errorf("engine: git branch -D %s: %w", branch, err)
	}
	if _, err := e.repo.Run(ctx, "update-ref", "-d", fmt.Sprintf("refs/stack-parent/%s", branch)); err != nil {
		return fmt.Errorf("engine: remove stack parent ref for %s: %w", branch, err)
	}
	return nil
}

func (e *Engine) cleanupStackRefs(ctx context.Context) error {
	branches, err := e.repo.ListBranches(ctx)
	if err != nil {
		return fmt.Errorf("engine: list branches: %w", err)
	}
	existing := make(map[string]struct{}, len(branches))
	for _, name := range branches {
		name = strings.TrimSpace(name)
		if name != "" {
			existing[name] = struct{}{}
		}
	}

	bottomRefs, err := e.repo.Run(ctx, "for-each-ref", "--format=%(refname:short)", stackyBottomRefPrefix)
	if err != nil {
		return fmt.Errorf("engine: list stacky bottom refs: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(bottomRefs), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		branch := strings.TrimPrefix(line, stackyBottomShortPrefix)
		if branch == "" {
			continue
		}
		if _, ok := existing[branch]; ok {
			continue
		}
		ref := fmt.Sprintf("%s%s", stackyBottomRefPrefix, branch)
		if _, err := e.repo.Run(ctx, "update-ref", "-d", ref); err != nil {
			return fmt.Errorf("engine: remove %s: %w", ref, err)
		}
	}

	parentRefs, err := e.repo.Run(ctx, "for-each-ref", "--format=%(refname:short)", "refs/stack-parent")
	if err != nil {
		return fmt.Errorf("engine: list stack parent refs: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(parentRefs), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		branch := strings.TrimPrefix(line, "stack-parent/")
		if branch == "" {
			continue
		}
		if _, ok := existing[branch]; ok {
			continue
		}
		ref := fmt.Sprintf("refs/stack-parent/%s", branch)
		if _, err := e.repo.Run(ctx, "update-ref", "-d", ref); err != nil {
			return fmt.Errorf("engine: remove stack parent ref for %s: %w", branch, err)
		}
	}

	return nil
}

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

	if st.Branch == "" {
		return "", fmt.Errorf("engine: stored branch state missing branch name")
	}

	if len(st.Sync) > 0 {
		if err := e.repo.Checkout(ctx, st.Branch); err != nil {
			return "", err
		}
		graph, err := e.loadGraph(ctx)
		if err != nil {
			return "", err
		}
		syncBranches := make([]*stackgraph.Branch, 0, len(st.Sync))
		for _, name := range st.Sync {
			branch, ok := graph.Branch(name)
			if !ok {
				return "", fmt.Errorf("engine: branch %s not found", name)
			}
			syncBranches = append(syncBranches, branch)
		}
		if err := e.runSync(ctx, st.Branch, syncBranches); err != nil {
			return "", err
		}
		return st.Branch, nil
	}

	if st.Fold != nil {
		if _, err := e.Branch(ctx, st.Branch); err != nil {
			return "", err
		}
		if err := e.repo.Checkout(ctx, st.Branch); err != nil {
			return "", err
		}
		if err := e.resumeCherryPickFold(ctx, st.Branch, st.Fold); err != nil {
			return "", err
		}
		return st.Branch, nil
	}

	if st.MergeFold != nil {
		if _, err := e.Branch(ctx, st.Branch); err != nil {
			return "", err
		}
		if err := e.repo.Checkout(ctx, st.Branch); err != nil {
			return "", err
		}
		if err := e.resumeMergeFold(ctx, st.MergeFold); err != nil {
			return "", err
		}
		return st.Branch, nil
	}

	if _, err := e.Branch(ctx, st.Branch); err != nil {
		return "", err
	}

	if err := e.Checkout(ctx, st.Branch); err != nil {
		return "", err
	}

	if err := e.clearState(); err != nil {
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

// UpstackOnto restacks the current branch and its descendants onto target.
func (e *Engine) UpstackOnto(ctx context.Context, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("engine: target branch must not be empty")
	}

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
		return fmt.Errorf("engine: may not restack stack bottom %s", branch.Name)
	}

	if target == branch.Name {
		return fmt.Errorf("engine: cannot restack branch %s onto itself", branch.Name)
	}

	targetBranch, ok := graph.Branch(target)
	if !ok {
		return fmt.Errorf("engine: target branch %s not found", target)
	}

	upstack, err := graph.Upstack(branch.Name)
	if err != nil {
		return err
	}
	for _, node := range upstack {
		if node.Name == targetBranch.Name {
			return fmt.Errorf("engine: target branch %s is upstack of %s", targetBranch.Name, branch.Name)
		}
	}

	if _, err := e.repo.Run(ctx, "config", fmt.Sprintf("branch.%s.merge", branch.Name), fmt.Sprintf("refs/heads/%s", targetBranch.Name)); err != nil {
		return fmt.Errorf("engine: git config branch.%s.merge: %w", branch.Name, err)
	}

	graph, err = e.loadGraph(ctx)
	if err != nil {
		return err
	}

	branch, ok = graph.Branch(current)
	if !ok {
		return fmt.Errorf("engine: branch %s not found", current)
	}

	return e.syncForest(ctx, []*stackgraph.Branch{branch}, current)
}

// UpstackAs promotes the current branch to a new stack bottom when target is "bottom".
func (e *Engine) UpstackAs(ctx context.Context, target string) error {
	mode := strings.ToLower(strings.TrimSpace(target))
	switch mode {
	case "bottom", "base":
	default:
		return fmt.Errorf("engine: invalid target %s, acceptable targets are [bottom]", target)
	}

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
		return fmt.Errorf("engine: branch %s is already a stack bottom", branch.Name)
	}

	if _, err := e.repo.Run(ctx, "config", fmt.Sprintf("branch.%s.merge", branch.Name), fmt.Sprintf("refs/heads/%s", branch.Name)); err != nil {
		return fmt.Errorf("engine: git config branch.%s.merge: %w", branch.Name, err)
	}

	ref := fmt.Sprintf("refs/stack-parent/%s", branch.Name)
	if _, err := e.repo.Run(ctx, "update-ref", "-d", ref); err != nil {
		return fmt.Errorf("engine: git update-ref -d %s: %w", ref, err)
	}

	head, err := e.repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", branch.Name))
	if err != nil {
		return err
	}
	head = strings.TrimSpace(head)
	if head == "" {
		return fmt.Errorf("engine: branch %s has no commit", branch.Name)
	}

	stackBottomRef := stackyBottomRefPrefix + branch.Name
	if err := e.repo.WriteRef(ctx, stackBottomRef, head, ""); err != nil {
		return fmt.Errorf("engine: update %s: %w", stackBottomRef, err)
	}

	return nil
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

// UpdatePRBody updates the description for the specified pull request.
func (e *Engine) UpdatePRBody(ctx context.Context, number int, body string) error {
	return e.gh.EditPRBody(ctx, number, body)
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (e *Engine) stackBottomNames(ctx context.Context) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(defaultStackBottoms))
	for _, name := range defaultStackBottoms {
		set[name] = struct{}{}
	}

	out, err := e.repo.Run(ctx, "for-each-ref", "--format=%(refname:short)", stackyBottomRefPrefix)
	if err != nil {
		return nil, fmt.Errorf("engine: list stacky bottom refs: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, stackyBottomShortPrefix)
		if line == "" {
			continue
		}
		set[line] = struct{}{}
	}

	return set, nil
}

func (e *Engine) realStackBottom(ctx context.Context, candidates map[string]struct{}) (string, error) {
	branches, err := e.repo.ListBranches(ctx)
	if err != nil {
		return "", fmt.Errorf("engine: list branches: %w", err)
	}
	matches := make([]string, 0, len(branches))
	for _, name := range branches {
		if _, ok := candidates[name]; ok {
			matches = append(matches, name)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", nil
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
