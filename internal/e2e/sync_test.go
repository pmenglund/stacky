package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStackSyncRebasesStack(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature\n"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "child.txt"), []byte("child\n"), 0o644))
	runGit(t, repoDir, "add", "child.txt")
	runGit(t, repoDir, "commit", "-m", "child change")

	oldMainHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/heads/main"))
	oldParentRef := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature"))
	require.Equal(t, oldMainHead, oldParentRef)

	runGit(t, repoDir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.txt"), []byte("main update\n"), 0o644))
	runGit(t, repoDir, "add", "main.txt")
	runGit(t, repoDir, "commit", "-m", "main update")

	newMainHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/heads/main"))
	require.NotEqual(t, oldMainHead, newMainHead)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "checkout", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "stack", "sync")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	parentRef := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature"))
	require.Equal(t, newMainHead, parentRef)

	featureHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/heads/feature"))
	childParent := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature-child"))
	require.Equal(t, featureHead, childParent)

	mergeBase := strings.TrimSpace(runGit(t, repoDir, "merge-base", "feature", "main"))
	require.Equal(t, newMainHead, mergeBase)

	childMergeBase := strings.TrimSpace(runGit(t, repoDir, "merge-base", "feature-child", "feature"))
	require.Equal(t, featureHead, childMergeBase)

	currentBranch := strings.TrimSpace(runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "feature-child", currentBranch)
}
