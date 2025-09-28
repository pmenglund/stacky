package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogRespectsUseMergeConfig(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()
	t.Setenv("GIT_PAGER", "cat")

	runGit(t, repoDir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature\n"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")
	runGit(t, repoDir, "checkout", "main")
	runGit(t, repoDir, "merge", "--no-ff", "feature")

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "log")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, runGit(t, repoDir, "log"), stdout)

	configContent := "[GIT]\nuse_merge = true\n"
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".stackyconfig"), []byte(configContent), 0o644))

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "log")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, runGit(t, repoDir, "log", "--no-merges", "--first-parent"), stdout)
}
