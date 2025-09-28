package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDownstackInfoShowsAncestorsPath(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	featureFile := filepath.Join(repoDir, "feature.txt")
	require.NoError(t, os.WriteFile(featureFile, []byte("feature\n"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	childFile := filepath.Join(repoDir, "child.txt")
	require.NoError(t, os.WriteFile(childFile, []byte("child\n"), 0o644))
	runGit(t, repoDir, "add", "child.txt")
	runGit(t, repoDir, "commit", "-m", "child change")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "checkout", "main")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "peer")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "checkout", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "downstack", "info")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "- main\n")
	require.Contains(t, stdout, "  - feature\n")
	require.Contains(t, stdout, "    - feature-child *\n")
	require.NotContains(t, stdout, "peer")
}
