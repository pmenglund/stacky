package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdoptWiresExistingBranchIntoStack(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	runGit(t, repoDir, "checkout", "-b", "topic")

	topicFile := filepath.Join(repoDir, "topic.txt")
	require.NoError(t, os.WriteFile(topicFile, []byte("topic change\n"), 0o644))
	runGit(t, repoDir, "add", "topic.txt")
	runGit(t, repoDir, "commit", "-m", "topic change")

	runGit(t, repoDir, "checkout", "main")

	mainFile := filepath.Join(repoDir, "main.txt")
	require.NoError(t, os.WriteFile(mainFile, []byte("main change\n"), 0o644))
	runGit(t, repoDir, "add", "main.txt")
	runGit(t, repoDir, "commit", "-m", "main change")

	mergeBase := strings.TrimSpace(runGit(t, repoDir, "merge-base", "main", "topic"))

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "--color=never", "adopt", "topic")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	parentRef := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/topic"))
	require.Equal(t, mergeBase, parentRef)

	remoteConfig := strings.TrimSpace(runGit(t, repoDir, "config", "branch.topic.remote"))
	require.Equal(t, ".", remoteConfig)

	mergeConfig := strings.TrimSpace(runGit(t, repoDir, "config", "branch.topic.merge"))
	require.Equal(t, "refs/heads/main", mergeConfig)

	currentBranch := strings.TrimSpace(runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "main", currentBranch)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "stack", "info")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "- main")
	require.Contains(t, stdout, "topic")
}
