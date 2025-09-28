package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFoldCherryPicksCommitsAndDeletesBranch(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature change\n"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "child.txt"), []byte("child change\n"), 0o644))
	runGit(t, repoDir, "add", "child.txt")
	runGit(t, repoDir, "commit", "-m", "child change")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "fold")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	head := strings.TrimSpace(runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "feature", head)

	contents, err := os.ReadFile(filepath.Join(repoDir, "child.txt"))
	require.NoError(t, err)
	require.Equal(t, "child change\n", string(contents))

	status := strings.TrimSpace(runGit(t, repoDir, "status", "--short"))
	require.Equal(t, "", status)

	branches := runGit(t, repoDir, "branch", "--list")
	require.NotContains(t, branches, "feature-child")

	cmd := exec.Command("git", "rev-parse", "refs/stack-parent/feature-child")
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	require.Error(t, cmd.Run())

	_, err = os.Stat(filepath.Join(homeDir, ".stacky.state"))
	require.True(t, os.IsNotExist(err))
}
