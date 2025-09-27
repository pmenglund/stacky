package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommitCommandCreatesCommit(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	withWorkingDir(t, repoDir, func() {
		// Move to a stack branch so commits are allowed.
		branchCmd := newBranchNewCmd()
		branchCmd.SetContext(context.Background())
		require.NoError(t, branchCmd.RunE(branchCmd, []string{"feature"}))

		// Create a change that will be staged by --add-all.
		content := []byte("initial feature work\n")
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), content, 0o644))

		commitCmd := newCommitCmd()
		commitCmd.SetContext(context.Background())
		commitCmd.SetOut(&bytes.Buffer{})
		commitCmd.SetErr(&bytes.Buffer{})
		require.NoError(t, commitCmd.Flags().Set("add-all", "true"))
		require.NoError(t, commitCmd.Flags().Set("message", "Add feature"))

		require.NoError(t, commitCmd.RunE(commitCmd, nil))

		logMsg := strings.TrimSpace(runGit(t, repoDir, "log", "-1", "--pretty=%B"))
		require.Equal(t, "Add feature", logMsg)
	})
}

func TestCommitCommandRejectsNoEditWithoutAmend(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	withWorkingDir(t, repoDir, func() {
		branchCmd := newBranchNewCmd()
		branchCmd.SetContext(context.Background())
		require.NoError(t, branchCmd.RunE(branchCmd, []string{"feature"}))

		commitCmd := newCommitCmd()
		commitCmd.SetContext(context.Background())
		commitCmd.SetOut(&bytes.Buffer{})
		commitCmd.SetErr(&bytes.Buffer{})
		require.NoError(t, commitCmd.Flags().Set("allow-empty", "true"))
		require.NoError(t, commitCmd.Flags().Set("no-edit", "true"))

		err := commitCmd.RunE(commitCmd, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no-edit")
	})
}

func TestAmendCommandAmendsCommit(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	withWorkingDir(t, repoDir, func() {
		branchCmd := newBranchNewCmd()
		branchCmd.SetContext(context.Background())
		require.NoError(t, branchCmd.RunE(branchCmd, []string{"feature"}))

		// Create an initial commit via the commit command.
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("v1\n"), 0o644))
		commitCmd := newCommitCmd()
		commitCmd.SetContext(context.Background())
		commitCmd.SetOut(&bytes.Buffer{})
		commitCmd.SetErr(&bytes.Buffer{})
		require.NoError(t, commitCmd.Flags().Set("add-all", "true"))
		require.NoError(t, commitCmd.Flags().Set("message", "Initial feature"))
		require.NoError(t, commitCmd.RunE(commitCmd, nil))

		original := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

		// Stage a change and amend without editing the message.
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("v2\n"), 0o644))
		runGit(t, repoDir, "add", "feature.txt")

		amendCmd := newAmendCmd()
		amendCmd.SetContext(context.Background())
		amendCmd.SetOut(&bytes.Buffer{})
		amendCmd.SetErr(&bytes.Buffer{})
		require.NoError(t, amendCmd.RunE(amendCmd, nil))

		updated := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
		require.NotEqual(t, original, updated)
		message := strings.TrimSpace(runGit(t, repoDir, "log", "-1", "--pretty=%B"))
		require.Equal(t, "Initial feature", message)
	})
}

