package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDownMovesToParentBranch(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	setupStackWithChild(t, repoDir, homeDir)

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "--color=never", "down")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "Checked out feature\n", stdout)

	head := strings.TrimSpace(runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "feature", head)
}

func TestDownFailsAtBottom(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	setupStackWithChild(t, repoDir, homeDir)

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "checkout", "main")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "down")
	require.Error(t, err)
	require.Contains(t, err.Error(), "already at stack bottom")
	require.Contains(t, stderr, "already at stack bottom")
	require.True(t, stdout == "" || strings.Contains(stdout, "Usage:"), "unexpected stdout: %q", stdout)
}

func TestUpMovesToChildBranch(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	setupStackWithChild(t, repoDir, homeDir)

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "checkout", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "up")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "Checked out feature-child\n", stdout)

	head := strings.TrimSpace(runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "feature-child", head)
}

func TestUpFailsWithoutChildren(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	setupStackWithChild(t, repoDir, homeDir)

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "checkout", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "up")
	require.Error(t, err)
	require.Contains(t, err.Error(), "has no children")
	require.Contains(t, stderr, "has no children")
	require.True(t, stdout == "" || strings.Contains(stdout, "Usage:"), "unexpected stdout: %q", stdout)
}

func setupStackWithChild(t *testing.T, repoDir, homeDir string) {
	t.Helper()

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	featurePath := filepath.Join(repoDir, "feature.txt")
	require.NoError(t, os.WriteFile(featurePath, []byte("feature\n"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	childPath := filepath.Join(repoDir, "child.txt")
	require.NoError(t, os.WriteFile(childPath, []byte("child\n"), 0o644))
	runGit(t, repoDir, "add", "child.txt")
	runGit(t, repoDir, "commit", "-m", "child change")
}
