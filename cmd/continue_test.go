package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestContinueCommandResumesSyncState(t *testing.T) {
	repoDir := initGitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_EDITOR", "true")
	t.Setenv("GIT_SEQUENCE_EDITOR", "true")

	withWorkingDir(t, repoDir, func() {
		newCmd := newBranchNewCmd()
		newCmd.SetContext(context.Background())
		require.NoError(t, newCmd.RunE(newCmd, []string{"feature"}))
		runGit(t, repoDir, "checkout", "main")
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.txt"), []byte("main change\n"), 0o644))
		runGit(t, repoDir, "add", "main.txt")
		runGit(t, repoDir, "commit", "-m", "main change")
	})

	require.NoError(t, state.Save(filepath.Join(home, ".stacky.state"), state.State{Branch: "feature", Sync: []string{"feature"}}))

	cmd := newContinueCmd()
	cmd.SetContext(context.Background())
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})

	withWorkingDir(t, repoDir, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})

	require.Contains(t, out.String(), "feature")
	parentRef := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature"))
	mainHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "main"))
	require.Equal(t, mainHead, parentRef)
	statePath := filepath.Join(home, ".stacky.state")
	_, err := os.Stat(statePath)
	require.True(t, os.IsNotExist(err))
}
