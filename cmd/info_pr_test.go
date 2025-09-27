package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/engine"
	"github.com/pmenglund/stacky/internal/githubcli"
	"github.com/pmenglund/stacky/internal/ui"
)

type fakeGitHubClient struct {
	prs []githubcli.PullRequest
	err error
}

func (f *fakeGitHubClient) ListPRs(_ context.Context, _ githubcli.ListParams) ([]githubcli.PullRequest, error) {
	if f.err != nil {
		return nil, f.err
	}
	result := make([]githubcli.PullRequest, len(f.prs))
	copy(result, f.prs)
	return result, nil
}

func (f *fakeGitHubClient) CreatePR(context.Context, githubcli.CreateParams) error { return nil }

func (f *fakeGitHubClient) EditPRBase(context.Context, int, string) error { return nil }

func runInfoCommandWithPRs(t *testing.T, factory func() *cobra.Command, repoDir, home string, prs []githubcli.PullRequest) string {
	t.Helper()

	ctx := withEngineOptions(context.Background(), engine.Options{
		RepoPath: repoDir,
		HomeDir:  home,
		GitHub:   &fakeGitHubClient{prs: prs},
	})

	cmd := factory()
	cmd.SetContext(ctx)

	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, cmd.Flags().Set("pr", "true"))

	withWorkingDir(t, repoDir, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})

	return out.String()
}

func TestStackInfoIncludesPRAnnotations(t *testing.T) {
	repoDir := initGitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	withWorkingDir(t, repoDir, func() {
		branchCmd := newBranchNewCmd()
		branchCmd.SetContext(context.Background())
		require.NoError(t, branchCmd.RunE(branchCmd, []string{"feature"}))
	})

	prev := colorMode
	colorMode = string(ui.ColorModeNever)
	defer func() { colorMode = prev }()

	output := runInfoCommandWithPRs(t, newStackInfoCmd, repoDir, home, []githubcli.PullRequest{{Number: 42, HeadRef: "feature"}})

	require.Contains(t, output, "feature")
	require.Contains(t, output, "(#42)")
}

func TestUpstackInfoIncludesPRAnnotations(t *testing.T) {
	repoDir := initGitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	withWorkingDir(t, repoDir, func() {
		branchCmd := newBranchNewCmd()
		branchCmd.SetContext(context.Background())
		require.NoError(t, branchCmd.RunE(branchCmd, []string{"feature"}))

		childCmd := newBranchNewCmd()
		childCmd.SetContext(context.Background())
		require.NoError(t, childCmd.RunE(childCmd, []string{"feature-child"}))

		runGit(t, repoDir, "checkout", "feature")
	})

	prev := colorMode
	colorMode = string(ui.ColorModeNever)
	defer func() { colorMode = prev }()

	prs := []githubcli.PullRequest{
		{Number: 101, HeadRef: "feature"},
		{Number: 102, HeadRef: "feature-child"},
	}

	output := runInfoCommandWithPRs(t, newUpstackInfoCmd, repoDir, home, prs)

	require.Contains(t, output, "feature")
	require.Contains(t, output, "feature-child")
	require.Contains(t, output, "(#101)")
	require.Contains(t, output, "(#102)")
}

func TestDownstackInfoIncludesPRAnnotations(t *testing.T) {
	repoDir := initGitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	withWorkingDir(t, repoDir, func() {
		branchCmd := newBranchNewCmd()
		branchCmd.SetContext(context.Background())
		require.NoError(t, branchCmd.RunE(branchCmd, []string{"feature"}))

		childCmd := newBranchNewCmd()
		childCmd.SetContext(context.Background())
		require.NoError(t, childCmd.RunE(childCmd, []string{"feature-child"}))
	})

	prev := colorMode
	colorMode = string(ui.ColorModeNever)
	defer func() { colorMode = prev }()

	prs := []githubcli.PullRequest{
		{Number: 201, HeadRef: "feature"},
		{Number: 202, HeadRef: "feature-child"},
	}

	withWorkingDir(t, repoDir, func() {
		runGit(t, repoDir, "checkout", "feature-child")
	})

	output := runInfoCommandWithPRs(t, newDownstackInfoCmd, repoDir, home, prs)

	require.Contains(t, output, "feature")
	require.Contains(t, output, "feature-child")
	require.Contains(t, output, "(#201)")
	require.Contains(t, output, "(#202)")
}
