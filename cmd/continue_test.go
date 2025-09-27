package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/state"
)

func TestContinueCommandResumesBranch(t *testing.T) {
	repoDir := initGitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	withWorkingDir(t, repoDir, func() {
		runGit(t, repoDir, "checkout", "-b", "feature")
		runGit(t, repoDir, "checkout", "main")
	})

	require.NoError(t, state.Save(filepath.Join(home, ".stacky.state"), state.State{Branch: "feature"}))

	cmd := newContinueCmd()
	cmd.SetContext(context.Background())
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})

	withWorkingDir(t, repoDir, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})

	require.Contains(t, out.String(), "feature")
	head := runGit(t, repoDir, "symbolic-ref", "--short", "HEAD")
	require.Equal(t, "feature\n", head)
}

func TestContinueCommandErrorsWithoutState(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	cmd := newContinueCmd()
	cmd.SetContext(context.Background())

	withWorkingDir(t, repoDir, func() {
		err := cmd.RunE(cmd, nil)
		require.Error(t, err)
	})
}

func TestContinueCommandErrorsForUnsupportedState(t *testing.T) {
	repoDir := initGitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	require.NoError(t, state.Save(filepath.Join(home, ".stacky.state"), state.State{Sync: []string{"feature"}}))

	cmd := newContinueCmd()
	cmd.SetContext(context.Background())

	withWorkingDir(t, repoDir, func() {
		err := cmd.RunE(cmd, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not implemented")
	})
}
