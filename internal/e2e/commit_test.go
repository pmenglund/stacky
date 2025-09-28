package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommitCreatesCommitWithAddAll(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature change\n"), 0o644))

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "commit", "-a", "-m", "feat: add feature change")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	message := strings.TrimSpace(runGit(t, repoDir, "log", "-1", "--pretty=%s"))
	require.Equal(t, "feat: add feature change", message)

	status := strings.TrimSpace(runGit(t, repoDir, "status", "--short"))
	require.Equal(t, "", status)

	current := strings.TrimSpace(runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "feature", current)
}

func TestCommitFailsWhenParentNotSynced(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)

	runGit(t, repoDir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.txt"), []byte("main change\n"), 0o644))
	runGit(t, repoDir, "add", "main.txt")
	runGit(t, repoDir, "commit", "-m", "main diverges")
	runGit(t, repoDir, "checkout", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature work\n"), 0o644))
	beforeMessage := strings.TrimSpace(runGit(t, repoDir, "log", "-1", "--pretty=%s"))

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "commit", "-a", "-m", "feat: should fail")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not synced with parent")
	require.Contains(t, stderr, "not synced with parent")
	require.True(t, strings.Contains(stdout, "Usage:") || stdout == "", "expected usage output in stdout, got %q", stdout)

	message := strings.TrimSpace(runGit(t, repoDir, "log", "-1", "--pretty=%s"))
	require.Equal(t, beforeMessage, message)
}
