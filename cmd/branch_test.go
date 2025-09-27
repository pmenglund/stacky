package cmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBranchCheckoutWithArgument(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	runGit(t, repoDir, "checkout", "-b", "feature")

	cmd := newBranchCheckoutCmd()
	cmd.SetArgs([]string{"main"})
	cmd.SetContext(context.Background())

	withWorkingDir(t, repoDir, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"main"}))
	})

	head := runGit(t, repoDir, "symbolic-ref", "--short", "HEAD")
	require.Equal(t, "main\n", head)
}

func TestBranchCheckoutInteractiveSelection(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	runGit(t, repoDir, "checkout", "-b", "feature")
	runGit(t, repoDir, "checkout", "main")

	cmd := newBranchCheckoutCmd()
	cmd.SetContext(context.Background())
	buf := &bytes.Buffer{}
	cmd.SetIn(strings.NewReader("1\n"))
	cmd.SetOut(buf)

	withWorkingDir(t, repoDir, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})

	head := runGit(t, repoDir, "symbolic-ref", "--short", "HEAD")
	require.Equal(t, "feature\n", head)
}

func TestBranchCommitCreatesBranchAndCommit(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	withWorkingDir(t, repoDir, func() {
		require.NoError(t, os.WriteFile("feature.txt", []byte("work"), 0o644))

		cmd := newBranchCommitCmd()
		cmd.SetContext(context.Background())
		cmd.Flags().Set("add-all", "true")
		cmd.Flags().Set("message", "feat: add work")

		require.NoError(t, cmd.RunE(cmd, []string{"feature"}))

		head := runGit(t, repoDir, "symbolic-ref", "--short", "HEAD")
		require.Equal(t, "feature\n", head)

		log := runGit(t, repoDir, "log", "-1", "--pretty=%s")
		require.Equal(t, "feat: add work\n", log)
	})
}

func TestBranchCommitAllowEmpty(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	withWorkingDir(t, repoDir, func() {
		cmd := newBranchCommitCmd()
		cmd.SetContext(context.Background())
		cmd.Flags().Set("allow-empty", "true")
		cmd.Flags().Set("message", "chore: empty commit")

		require.NoError(t, cmd.RunE(cmd, []string{"feature"}))

		head := runGit(t, repoDir, "symbolic-ref", "--short", "HEAD")
		require.Equal(t, "feature\n", head)

		log := runGit(t, repoDir, "log", "-1", "--pretty=%s")
		require.Equal(t, "chore: empty commit\n", log)
	})
}

func TestBranchCheckoutFailsWhenOnlyCurrentBranch(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	cmd := newBranchCheckoutCmd()
	cmd.SetContext(context.Background())
	buf := &bytes.Buffer{}
	cmd.SetIn(strings.NewReader("1\n"))
	cmd.SetOut(buf)

	withWorkingDir(t, repoDir, func() {
		err := cmd.RunE(cmd, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no other branches")
	})
}

func TestBranchCheckoutInteractiveChoosesNamedOption(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	withWorkingDir(t, repoDir, func() {
		newCmd := newBranchNewCmd()
		newCmd.SetContext(context.Background())
		require.NoError(t, newCmd.RunE(newCmd, []string{"feature"}))
		runGit(t, repoDir, "checkout", "main")
		newCmd = newBranchNewCmd()
		newCmd.SetContext(context.Background())
		require.NoError(t, newCmd.RunE(newCmd, []string{"bugfix"}))
		runGit(t, repoDir, "checkout", "main")

		cmd := newBranchCheckoutCmd()
		cmd.SetContext(context.Background())
		buf := &bytes.Buffer{}
		cmd.SetIn(strings.NewReader("bugfix\n"))
		cmd.SetOut(buf)

		require.NoError(t, cmd.RunE(cmd, nil))

		head := runGit(t, repoDir, "symbolic-ref", "--short", "HEAD")
		require.Equal(t, "bugfix\n", head)
	})
}

// shared helpers are defined in test_helpers_test.go
