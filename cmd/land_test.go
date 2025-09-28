package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/engine"
	"github.com/pmenglund/stacky/internal/githubcli"
)

type fakeLandGitHub struct {
	prs    []githubcli.PullRequest
	merges []githubcli.MergeParams
}

func (f *fakeLandGitHub) ListPRs(_ context.Context, _ githubcli.ListParams) ([]githubcli.PullRequest, error) {
	result := make([]githubcli.PullRequest, len(f.prs))
	copy(result, f.prs)
	return result, nil
}

func (f *fakeLandGitHub) CreatePR(context.Context, githubcli.CreateParams) error { return nil }

func (f *fakeLandGitHub) EditPRBase(context.Context, int, string) error { return nil }

func (f *fakeLandGitHub) EditPRBody(context.Context, int, string) error { return nil }

func (f *fakeLandGitHub) MergePR(_ context.Context, params githubcli.MergeParams) error {
	f.merges = append(f.merges, params)
	return nil
}

func (f *fakeLandGitHub) GetPRForBranch(_ context.Context, params githubcli.GetPRParams) (githubcli.BranchPullRequests, error) {
	all := make(map[string]githubcli.PullRequest)
	var open *githubcli.PullRequest
	for i := range f.prs {
		pr := f.prs[i]
		if params.Branch != "" && pr.HeadRef != params.Branch {
			continue
		}
		if pr.ID != "" {
			all[pr.ID] = pr
		}
		if open == nil {
			copy := pr
			open = &copy
		}
	}
	return githubcli.BranchPullRequests{All: all, Open: open}, nil
}

func (f *fakeLandGitHub) UpdateReviewers(context.Context, githubcli.UpdateReviewersParams) error {
	return nil
}

func setupLandRepo(t *testing.T) (repoDir, home string) {
	repoDir = initGitRepo(t)
	home = t.TempDir()
	t.Setenv("HOME", home)

	withWorkingDir(t, repoDir, func() {
		runGit(t, repoDir, "checkout", "-b", "feature")
		runGit(t, repoDir, "config", "branch.feature.merge", "refs/heads/main")
		runGit(t, repoDir, "config", "branch.feature.remote", ".")
		mainHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "main"))
		runGit(t, repoDir, "update-ref", "refs/stack-parent/feature", mainHead)
		runGit(t, repoDir, "checkout", "main")
	})

	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	require.NoError(t, os.MkdirAll(remoteDir, 0o755))
	runGit(t, remoteDir, "init", "--bare")

	withWorkingDir(t, repoDir, func() {
		runGit(t, repoDir, "remote", "add", "origin", remoteDir)
		runGit(t, repoDir, "push", "origin", "main")
		runGit(t, repoDir, "push", "origin", "feature")
		runGit(t, repoDir, "checkout", "feature")
	})

	return repoDir, home
}

func TestLandCommandMergesPullRequest(t *testing.T) {
	repoDir, home := setupLandRepo(t)
	fake := &fakeLandGitHub{prs: []githubcli.PullRequest{{HeadRef: "feature", Number: 17, URL: "https://example.com/pr", Mergeable: "MERGEABLE"}}}

	ctx := withEngineOptions(context.Background(), engine.Options{RepoPath: repoDir, HomeDir: home, GitHub: fake})
	cmd := newLandCmd()
	cmd.SetContext(ctx)
	cmd.SetIn(bytes.NewBufferString("yes\n"))
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})

	withWorkingDir(t, repoDir, func() {
		require.NoError(t, cmd.Execute())
	})

	require.Contains(t, out.String(), "Will land PR #17")
	require.Contains(t, out.String(), "Success! Run `stacky update`")
	require.Len(t, fake.merges, 1)
	require.Equal(t, "feature", fake.merges[0].Head)
	require.NotEmpty(t, fake.merges[0].MatchHead)
	require.False(t, fake.merges[0].Auto)
}

func TestLandCommandHonorsAutoFlag(t *testing.T) {
	repoDir, home := setupLandRepo(t)
	fake := &fakeLandGitHub{prs: []githubcli.PullRequest{{HeadRef: "feature", Number: 21, Mergeable: "MERGEABLE"}}}

	ctx := withEngineOptions(context.Background(), engine.Options{RepoPath: repoDir, HomeDir: home, GitHub: fake})
	cmd := newLandCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--auto", "--force"})
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	withWorkingDir(t, repoDir, func() {
		require.NoError(t, cmd.Execute())
	})

	require.Len(t, fake.merges, 1)
	require.True(t, fake.merges[0].Auto)
}
