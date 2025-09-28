package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBranchNavigationMovesBetweenStackBranches(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	mainHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "main"))
	parentRef := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature"))
	require.Equal(t, mainHead, parentRef)

	featurePath := filepath.Join(repoDir, "feature.txt")
	require.NoError(t, os.WriteFile(featurePath, []byte("feature\n"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature commit")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	childParent := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature-child"))
	featureHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "feature"))
	require.Equal(t, featureHead, childParent)

	runGit(t, repoDir, "checkout", "feature")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "up")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "Checked out feature-child\n", stdout)

	head := strings.TrimSpace(runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "feature-child", head)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "down")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "Checked out feature\n", stdout)

	head = strings.TrimSpace(runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "feature", head)
}

func TestBranchUpChoosesLexicographicallyFirstChild(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-b")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	runGit(t, repoDir, "checkout", "feature")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-a")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	runGit(t, repoDir, "checkout", "feature")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "up")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "Checked out feature-a\n", stdout)

	head := strings.TrimSpace(runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "feature-a", head)
}

func TestBranchCommitCreatesBranchAndCommitsChanges(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	featurePath := filepath.Join(repoDir, "feature.txt")
	require.NoError(t, os.WriteFile(featurePath, []byte("feature work\n"), 0o644))

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "--color=never", "branch", "commit", "--add-all", "--message", "feat: add feature", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	head := strings.TrimSpace(runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "feature", head)

	message := strings.TrimSpace(runGit(t, repoDir, "log", "-1", "--pretty=%s"))
	require.Equal(t, "feat: add feature", message)

	parentRef := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature"))
	mainCommit := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "main"))
	require.Equal(t, mainCommit, parentRef)
}
