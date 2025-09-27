package engine_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/engine"
	"github.com/pmenglund/stacky/internal/githubcli"
	"github.com/pmenglund/stacky/internal/stackgraph"
	"github.com/pmenglund/stacky/internal/state"
)

type fakeGitHub struct {
	prs     []githubcli.PullRequest
	err     error
	params  []githubcli.ListParams
	created []githubcli.CreateParams
	edits   []struct {
		number int
		base   string
	}
}

func (f *fakeGitHub) ListPRs(_ context.Context, params githubcli.ListParams) ([]githubcli.PullRequest, error) {
	f.params = append(f.params, params)
	if f.err != nil {
		return nil, f.err
	}
	return f.prs, nil
}

func (f *fakeGitHub) CreatePR(_ context.Context, params githubcli.CreateParams) error {
	if f.err != nil {
		return f.err
	}
	f.created = append(f.created, params)
	return nil
}

func (f *fakeGitHub) EditPRBase(_ context.Context, number int, base string) error {
	if f.err != nil {
		return f.err
	}
	f.edits = append(f.edits, struct {
		number int
		base   string
	}{number: number, base: base})
	return nil
}

func TestStackForestAndCurrentStackRoot(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	opts := engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()}
	eng, err := engine.New(ctx, opts)
	require.NoError(t, err)

	forest, err := eng.StackForest(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"main"}, names(forest))

	root, err := eng.CurrentStackRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, "main", root.Name)
}

func TestCreateBranchSetsParentRef(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	opts := engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()}
	eng, err := engine.New(ctx, opts)
	require.NoError(t, err)

	reqParent := gitRevParse(t, repoDir, "feature")

	require.NoError(t, eng.CreateBranch(ctx, "child"))

	childCommit := gitRevParse(t, repoDir, "child")
	require.Equal(t, reqParent, childCommit)

	parentRef := gitRevParse(t, repoDir, "refs/stack-parent/child")
	require.Equal(t, reqParent, parentRef)
}

func TestCommitCreatesNewCommit(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	opts := engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()}
	eng, err := engine.New(ctx, opts)
	require.NoError(t, err)

	runGit(t, repoDir, "checkout", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("changes"), 0o644))
	parentCommit := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "refs/stack-parent/feature"))
	mainHead := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "main"))
	require.Equal(t, mainHead, parentCommit)
	require.NoError(t, eng.Commit(ctx, engine.CommitOptions{AddAll: true, Message: "feat: add file"}))

	log := runGitOutput(t, repoDir, "log", "-1", "--pretty=%s")
	require.Equal(t, "feat: add file", strings.TrimSpace(log))
}

func TestLogRespectsUseMerge(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	// Add work on feature branch and merge into main to produce a merge commit.
	runGit(t, repoDir, "checkout", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature work")
	runGit(t, repoDir, "checkout", "main")
	runGit(t, repoDir, "merge", "--no-ff", "feature")

	defaultHome := t.TempDir()
	engDefault, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: defaultHome})
	require.NoError(t, err)

	logDefault, err := engDefault.Log(ctx)
	require.NoError(t, err)
	require.Equal(t, runGitOutput(t, repoDir, "log"), logDefault)

	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".stackyconfig"), []byte("[GIT]\nuse_merge = true\n"), 0o644))
	engMerge, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: home})
	require.NoError(t, err)

	logMerge, err := engMerge.Log(ctx)
	require.NoError(t, err)
	require.Equal(t, runGitOutput(t, repoDir, "log", "--no-merges", "--first-parent"), logMerge)
}

func TestContinueRestoresBranch(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	runGit(t, repoDir, "checkout", "main")

	home := t.TempDir()
	statePath := filepath.Join(home, ".stacky.state")
	require.NoError(t, state.Save(statePath, state.State{Branch: "feature"}))

	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: home})
	require.NoError(t, err)

	branch, err := eng.Continue(ctx)
	require.NoError(t, err)
	require.Equal(t, "feature", branch)

	head := runGitOutput(t, repoDir, "symbolic-ref", "--short", "HEAD")
	require.Equal(t, "feature\n", head)

	_, err = os.Stat(statePath)
	require.True(t, os.IsNotExist(err))
}

func TestContinueErrorsWhenSyncStatePresent(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	home := t.TempDir()
	statePath := filepath.Join(home, ".stacky.state")
	require.NoError(t, state.Save(statePath, state.State{Sync: []string{"feature"}}))

	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: home})
	require.NoError(t, err)

	_, err = eng.Continue(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not implemented")
}

func TestUpdateErrorsWhenRemoteMissing(t *testing.T) {
	ctx := context.Background()
	localDir := filepath.Join(t.TempDir(), "local")
	require.NoError(t, os.MkdirAll(localDir, 0o755))
	runGit(t, localDir, "init", "-b", "main")
	runGit(t, localDir, "config", "user.email", "engine@example.com")
	runGit(t, localDir, "config", "user.name", "Engine Tester")
	runGit(t, localDir, "config", "commit.gpgsign", "false")
	runGit(t, localDir, "config", "core.fsmonitor", "false")
	require.NoError(t, os.WriteFile(filepath.Join(localDir, "README.md"), []byte("local"), 0o644))
	runGit(t, localDir, "add", "README.md")
	runGit(t, localDir, "commit", "-m", "initial commit")

	home := t.TempDir()
	eng, err := engine.New(ctx, engine.Options{RepoPath: localDir, HomeDir: home})
	require.NoError(t, err)

	_, err = eng.Update(ctx, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "git fetch origin")
}

func TestCommitFailsOnBottomBranch(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()})
	require.NoError(t, err)

	// main is bottom branch
	err = eng.Commit(ctx, engine.CommitOptions{Message: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "do not commit directly")
}

func TestCommitFailsWhenNotSyncedWithParent(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()})
	require.NoError(t, err)

	runGit(t, repoDir, "checkout", "feature")
	// create new commit on parent to desync
	runGit(t, repoDir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.txt"), []byte("main"), 0o644))
	runGit(t, repoDir, "add", "main.txt")
	runGit(t, repoDir, "commit", "-m", "main change")
	runGit(t, repoDir, "checkout", "feature")

	err = eng.Commit(ctx, engine.CommitOptions{Message: "feat"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not synced with parent")
}

func TestCommitFailsWhenNoEditWithoutAmend(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()})
	require.NoError(t, err)

	runGit(t, repoDir, "checkout", "feature")
	err = eng.Commit(ctx, engine.CommitOptions{NoEdit: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--no-edit")
}

func TestFoldCherryPickReparentsChildren(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	runGit(t, repoDir, "checkout", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature change"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")

	runGit(t, repoDir, "checkout", "-b", "sub")
	runGit(t, repoDir, "config", "branch.sub.merge", "refs/heads/feature")
	runGit(t, repoDir, "config", "branch.sub.remote", ".")
	featureHead := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "feature"))
	runGit(t, repoDir, "update-ref", "refs/stack-parent/sub", featureHead)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "sub.txt"), []byte("sub change"), 0o644))
	runGit(t, repoDir, "add", "sub.txt")
	runGit(t, repoDir, "commit", "-m", "sub change")

	runGit(t, repoDir, "checkout", "-b", "subchild")
	runGit(t, repoDir, "config", "branch.subchild.merge", "refs/heads/sub")
	runGit(t, repoDir, "config", "branch.subchild.remote", ".")
	subHead := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "sub"))
	runGit(t, repoDir, "update-ref", "refs/stack-parent/subchild", subHead)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "subchild.txt"), []byte("subchild change"), 0o644))
	runGit(t, repoDir, "add", "subchild.txt")
	runGit(t, repoDir, "commit", "-m", "subchild change")

	runGit(t, repoDir, "checkout", "sub")

	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()})
	require.NoError(t, err)

	require.NoError(t, eng.Fold(ctx, false))

	head := strings.TrimSpace(runGitOutput(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "feature", head)

	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/sub")
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	err = cmd.Run()
	require.Error(t, err)

	mergeTarget := strings.TrimSpace(runGitOutput(t, repoDir, "config", "branch.subchild.merge"))
	require.Equal(t, "refs/heads/feature", mergeTarget)

	featureHeadAfter := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "feature"))
	subchildParent := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "refs/stack-parent/subchild"))
	require.Equal(t, featureHeadAfter, subchildParent)

	status := strings.TrimSpace(runGitOutput(t, repoDir, "status", "--short"))
	require.Equal(t, "", status)
}

func TestFoldErrorsWhenParentIsStackBottom(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	runGit(t, repoDir, "checkout", "feature")

	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()})
	require.NoError(t, err)

	err = eng.Fold(ctx, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "stack bottom")
}

func TestStackPushPushesBranches(t *testing.T) {
	ctx := context.Background()
	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	require.NoError(t, os.MkdirAll(remoteDir, 0o755))
	runGit(t, remoteDir, "init", "--bare")

	repoDir := initRepo(t)
	runGit(t, repoDir, "remote", "add", "origin", remoteDir)
	runGit(t, repoDir, "push", "-u", "origin", "main")

	runGit(t, repoDir, "checkout", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")

	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()})
	require.NoError(t, err)

	require.NoError(t, eng.StackPush(ctx, engine.PushOptions{Remote: "origin"}))

	remoteLine := strings.TrimSpace(runGitOutput(t, repoDir, "ls-remote", remoteDir, "refs/heads/feature"))
	require.NotEmpty(t, remoteLine)
	parts := strings.Fields(remoteLine)
	require.True(t, len(parts) >= 1)
	remoteHead := parts[0]
	localHead := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "feature"))
	require.Equal(t, localHead, remoteHead)
}

func TestPlanStackPushErrorsWhenUnsynced(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()})
	require.NoError(t, err)

	runGit(t, repoDir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "diverge.txt"), []byte("main"), 0o644))
	runGit(t, repoDir, "add", "diverge.txt")
	runGit(t, repoDir, "commit", "-m", "diverge")

	_, err = eng.PlanStackPush(ctx, "origin", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not synced with parent")
}

func TestPlanStackPushIncludesPRActions(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	fake := &fakeGitHub{prs: []githubcli.PullRequest{{HeadRef: "feature", Number: 7, BaseRef: "develop"}}}
	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir(), GitHub: fake})
	require.NoError(t, err)

	plan, err := eng.PlanStackPush(ctx, "origin", true)
	require.NoError(t, err)
	require.Len(t, plan.Actions, 2)
	var featureAction engine.PushAction
	for _, action := range plan.Actions {
		if action.Branch == "feature" {
			featureAction = action
		}
	}
	require.Equal(t, "main", featureAction.Parent)
	require.Equal(t, engine.PRActionUpdateBase, featureAction.PRAction)
	require.Equal(t, 7, featureAction.PRNumber)
}

func TestExecutePushPlanCreatesPR(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	fake := &fakeGitHub{}
	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir(), GitHub: fake})
	require.NoError(t, err)

	plan := engine.PushPlan{
		Remote:  "origin",
		Actions: []engine.PushAction{{Branch: "feature", Parent: "main", PRAction: engine.PRActionCreate}},
	}

	require.NoError(t, eng.ExecutePushPlan(ctx, plan))
	require.Equal(t, []githubcli.CreateParams{{Head: "feature", Base: "main"}}, fake.created)
}

func TestExecutePushPlanEditsPRBase(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	fake := &fakeGitHub{}
	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir(), GitHub: fake})
	require.NoError(t, err)

	plan := engine.PushPlan{
		Remote:  "origin",
		Actions: []engine.PushAction{{Branch: "feature", Parent: "main", PRAction: engine.PRActionUpdateBase, PRNumber: 42}},
	}

	require.NoError(t, eng.ExecutePushPlan(ctx, plan))
	require.Len(t, fake.edits, 1)
	require.Equal(t, 42, fake.edits[0].number)
	require.Equal(t, "main", fake.edits[0].base)
}

func TestOpenPullRequestsMapsByHead(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	fake := &fakeGitHub{prs: []githubcli.PullRequest{{HeadRef: "feature", Number: 42}}}
	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir(), GitHub: fake})
	require.NoError(t, err)

	prs, err := eng.OpenPullRequests(ctx)
	require.NoError(t, err)
	require.Equal(t, 42, prs["feature"].Number)
}

func TestOpenPullRequestsPropagatesError(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	fake := &fakeGitHub{err: errors.New("boom")}
	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir(), GitHub: fake})
	require.NoError(t, err)

	_, err = eng.OpenPullRequests(ctx)
	require.Error(t, err)
}

func TestAuthoredPullRequestsUsesAuthorFilter(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	fake := &fakeGitHub{prs: []githubcli.PullRequest{{Number: 1}}}
	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir(), GitHub: fake})
	require.NoError(t, err)

	prs, err := eng.AuthoredPullRequests(ctx)
	require.NoError(t, err)
	require.Len(t, prs, 1)
	require.Equal(t, "@me", fake.params[0].Author)
}

func TestReviewRequestedPullRequestsUsesSearch(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	fake := &fakeGitHub{prs: []githubcli.PullRequest{{Number: 2}}}
	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir(), GitHub: fake})
	require.NoError(t, err)

	prs, err := eng.ReviewRequestedPullRequests(ctx)
	require.NoError(t, err)
	require.Len(t, prs, 1)
	require.Equal(t, "review-requested:@me", fake.params[0].Search)
}

func TestStackSyncRebasesBranches(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	runGit(t, repoDir, "checkout", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")

	runGit(t, repoDir, "checkout", "-b", "sub")
	runGit(t, repoDir, "config", "branch.sub.merge", "refs/heads/feature")
	runGit(t, repoDir, "config", "branch.sub.remote", ".")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "sub.txt"), []byte("sub"), 0o644))
	runGit(t, repoDir, "add", "sub.txt")
	runGit(t, repoDir, "commit", "-m", "sub change")
	featureHead := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "feature"))
	runGit(t, repoDir, "update-ref", "refs/stack-parent/sub", featureHead)

	runGit(t, repoDir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.txt"), []byte("main"), 0o644))
	runGit(t, repoDir, "add", "main.txt")
	runGit(t, repoDir, "commit", "-m", "main change")

	runGit(t, repoDir, "checkout", "sub")

	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()})
	require.NoError(t, err)

	require.NoError(t, eng.StackSync(ctx))

	headBranch := strings.TrimSpace(runGitOutput(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "sub", headBranch)

	updatedFeatureParent := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "refs/stack-parent/sub"))
	updatedFeatureHead := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "feature"))
	require.Equal(t, updatedFeatureHead, updatedFeatureParent)

	status := strings.TrimSpace(runGitOutput(t, repoDir, "status", "--short"))
	require.Equal(t, "", status)
}

func TestDownstackSyncRebasesAncestors(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	runGit(t, repoDir, "checkout", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")

	runGit(t, repoDir, "checkout", "-b", "sub")
	runGit(t, repoDir, "config", "branch.sub.merge", "refs/heads/feature")
	runGit(t, repoDir, "config", "branch.sub.remote", ".")
	featureHead := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "feature"))
	runGit(t, repoDir, "update-ref", "refs/stack-parent/sub", featureHead)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "sub.txt"), []byte("sub"), 0o644))
	runGit(t, repoDir, "add", "sub.txt")
	runGit(t, repoDir, "commit", "-m", "sub change")

	runGit(t, repoDir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.txt"), []byte("main"), 0o644))
	runGit(t, repoDir, "add", "main.txt")
	runGit(t, repoDir, "commit", "-m", "main diverged")

	runGit(t, repoDir, "checkout", "sub")

	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()})
	require.NoError(t, err)

	require.NoError(t, eng.DownstackSync(ctx))

	headBranch := strings.TrimSpace(runGitOutput(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "sub", headBranch)

	updatedMainHead := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "main"))
	featureParent := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "refs/stack-parent/feature"))
	require.Equal(t, updatedMainHead, featureParent)

	updatedFeatureHead := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "feature"))
	subParent := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "refs/stack-parent/sub"))
	require.Equal(t, updatedFeatureHead, subParent)
}

func TestDownstackPushPushesAncestors(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	runGit(t, repoDir, "checkout", "feature")
	runGit(t, repoDir, "checkout", "-b", "sub")
	runGit(t, repoDir, "config", "branch.sub.merge", "refs/heads/feature")
	runGit(t, repoDir, "config", "branch.sub.remote", ".")
	featureHead := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "feature"))
	runGit(t, repoDir, "update-ref", "refs/stack-parent/sub", featureHead)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "sub.txt"), []byte("sub"), 0o644))
	runGit(t, repoDir, "add", "sub.txt")
	runGit(t, repoDir, "commit", "-m", "sub change")
	runGit(t, repoDir, "checkout", "sub")

	remoteDir := t.TempDir()
	runGit(t, remoteDir, "init", "--bare")
	runGit(t, repoDir, "remote", "add", "origin", remoteDir)

	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()})
	require.NoError(t, err)

	require.NoError(t, eng.DownstackPush(ctx, engine.PushOptions{}))

	_, err = os.Stat(filepath.Join(remoteDir, "refs", "heads", "feature"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(remoteDir, "refs", "heads", "sub"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(remoteDir, "refs", "heads", "main"))
	require.True(t, os.IsNotExist(err))
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "engine@example.com")
	runGit(t, dir, "config", "user.name", "Engine Tester")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	runGit(t, dir, "config", "core.fsmonitor", "false")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644))

	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")

	runGit(t, dir, "checkout", "-b", "feature")
	runGit(t, dir, "config", "branch.feature.merge", "refs/heads/main")
	runGit(t, dir, "config", "branch.feature.remote", ".")
	mainHead := strings.TrimSpace(runGitOutput(t, dir, "rev-parse", "main"))
	runGit(t, dir, "update-ref", "refs/stack-parent/feature", mainHead)
	runGit(t, dir, "checkout", "main")

	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	return string(out)
}

func gitRevParse(t *testing.T, dir, rev string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", rev)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git rev-parse %s: %s", rev, string(out))
	return strings.TrimSpace(string(out))
}

func names(branches []*stackgraph.Branch) []string {
	out := make([]string, len(branches))
	for i, b := range branches {
		out[i] = b.Name
	}
	return out
}
