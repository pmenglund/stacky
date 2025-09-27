package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootCheckoutWithArgument(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	runGit(t, repoDir, "checkout", "-b", "feature")
	runGit(t, repoDir, "checkout", "main")

	cmd := newRootCheckoutCmd()
	cmd.SetContext(context.Background())

	withWorkingDir(t, repoDir, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"feature"}))
	})

	head := runGit(t, repoDir, "symbolic-ref", "--short", "HEAD")
	require.Equal(t, "feature\n", head)
}

func TestRootCheckoutInteractiveSelection(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	runGit(t, repoDir, "checkout", "-b", "feature")
	runGit(t, repoDir, "checkout", "main")

	cmd := newRootCheckoutCmd()
	cmd.SetContext(context.Background())
	cmd.SetIn(strings.NewReader("1\n"))
	cmd.SetOut(&bytes.Buffer{})

	withWorkingDir(t, repoDir, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})

	head := runGit(t, repoDir, "symbolic-ref", "--short", "HEAD")
	require.Equal(t, "feature\n", head)
}
