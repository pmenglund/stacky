package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStackCheckoutValidBranch(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	withWorkingDir(t, repoDir, func() {
		// Create stack branch using CLI helper.
		newCmd := newBranchNewCmd()
		newCmd.SetContext(context.Background())
		require.NoError(t, newCmd.RunE(newCmd, []string{"feature"}))

		checkoutCmd := newStackCheckoutCmd()
		checkoutCmd.SetContext(context.Background())
		require.NoError(t, checkoutCmd.RunE(checkoutCmd, []string{"main"}))

		head := runGit(t, repoDir, "symbolic-ref", "--short", "HEAD")
		require.Equal(t, "main\n", head)
	})
}

func TestStackCheckoutRejectsUnknownBranch(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	withWorkingDir(t, repoDir, func() {
		newCmd := newBranchNewCmd()
		newCmd.SetContext(context.Background())
		require.NoError(t, newCmd.RunE(newCmd, []string{"feature"}))

		checkoutCmd := newStackCheckoutCmd()
		checkoutCmd.SetContext(context.Background())
		err := checkoutCmd.RunE(checkoutCmd, []string{"unknown"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not in the current stack")
	})
}

func TestStackCheckoutInteractiveSelection(t *testing.T) {
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

		checkoutCmd := newStackCheckoutCmd()
		checkoutCmd.SetContext(context.Background())
		buf := &bytes.Buffer{}
		checkoutCmd.SetIn(strings.NewReader("bugfix\n"))
		checkoutCmd.SetOut(buf)

		require.NoError(t, checkoutCmd.RunE(checkoutCmd, nil))

		head := runGit(t, repoDir, "symbolic-ref", "--short", "HEAD")
		require.Equal(t, "bugfix\n", head)
	})
}

func TestStackCheckoutErrorsWhenOnlyCurrentBranch(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	withWorkingDir(t, repoDir, func() {
		checkoutCmd := newStackCheckoutCmd()
		checkoutCmd.SetContext(context.Background())
		buf := &bytes.Buffer{}
		checkoutCmd.SetIn(strings.NewReader("1\n"))
		checkoutCmd.SetOut(buf)

		err := checkoutCmd.RunE(checkoutCmd, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no other branches")
	})
}

// shared helpers are defined in test_helpers_test.go
