package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContinueResumesStackSyncAfterConflict(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	t.Setenv("GIT_EDITOR", "true")
	t.Setenv("GIT_SEQUENCE_EDITOR", "true")

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("feature change\n"), 0o644))
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "feature change")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "child.txt"), []byte("child change\n"), 0o644))
	runGit(t, repoDir, "add", "child.txt")
	runGit(t, repoDir, "commit", "-m", "child change")

	runGit(t, repoDir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("main change\n"), 0o644))
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "main change")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "checkout", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "stack", "sync")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Resolve conflicts")
	require.Contains(t, stderr, "Resolve conflicts")

	statePath := filepath.Join(homeDir, ".stacky.state")
	_, statErr := os.Stat(statePath)
	require.NoError(t, statErr)

	contents, err := os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Contains(t, string(contents), "<<<<<<<")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("resolved content\n"), 0o644))
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "rebase", "--continue")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "continue")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Resumed on branch feature-child")

	_, err = os.Stat(statePath)
	require.True(t, os.IsNotExist(err))

	mainHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/heads/main"))
	featureParent := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature"))
	require.Equal(t, mainHead, featureParent)

	featureHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/heads/feature"))
	childParent := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature-child"))
	require.Equal(t, featureHead, childParent)

	current := strings.TrimSpace(runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "feature-child", current)

	status := strings.TrimSpace(runGit(t, repoDir, "status", "--short"))
	require.Equal(t, "", status)
}
