package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstackOntoRestacksCurrentBranch(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	featureFile := filepath.Join(repoDir, "feature.txt")
	require.NoError(t, os.WriteFile(featureFile, []byte("feature\n"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature commit")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	childFile := filepath.Join(repoDir, "child.txt")
	require.NoError(t, os.WriteFile(childFile, []byte("child\n"), 0o644))
	runGit(t, repoDir, "add", "child.txt")
	runGit(t, repoDir, "commit", "-m", "child commit")

	runGit(t, repoDir, "checkout", "main")
	mainUpdate := filepath.Join(repoDir, "main.txt")
	require.NoError(t, os.WriteFile(mainUpdate, []byte("main\n"), 0o644))
	runGit(t, repoDir, "add", "main.txt")
	runGit(t, repoDir, "commit", "-m", "main update")

	runGit(t, repoDir, "checkout", "feature")
	featureBefore := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "feature"))

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "upstack", "onto", "main")
	require.NoError(t, err)
	require.Empty(t, stderr)

	featureAfter := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "feature"))
	require.NotEqual(t, featureBefore, featureAfter)

	mainHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "main"))
	parentRef := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature"))
	require.Equal(t, mainHead, parentRef)

	mergeConfig := strings.TrimSpace(runGit(t, repoDir, "config", "branch.feature.merge"))
	require.Equal(t, "refs/heads/main", mergeConfig)

	childParent := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature-child"))
	require.Equal(t, featureAfter, childParent)

	mergeBase := strings.TrimSpace(runGit(t, repoDir, "merge-base", "feature", "main"))
	require.Equal(t, mainHead, mergeBase)
}

func TestUpstackAsPromotesBranchToBottom(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	featureFile := filepath.Join(repoDir, "feature.txt")
	require.NoError(t, os.WriteFile(featureFile, []byte("feature\n"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature commit")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "upstack", "as", "bottom")
	require.NoError(t, err)
	require.Empty(t, stderr)

	parentRefs := strings.TrimSpace(runGit(t, repoDir, "for-each-ref", "--format=%(refname)", "refs/stack-parent"))
	require.NotContains(t, parentRefs, "refs/stack-parent/feature")

	featureHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "feature"))
	bottomRef := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stacky-bottom-branch/feature"))
	require.Equal(t, featureHead, bottomRef)

	mergeConfig := strings.TrimSpace(runGit(t, repoDir, "config", "branch.feature.merge"))
	require.Equal(t, "refs/heads/feature", mergeConfig)
}
