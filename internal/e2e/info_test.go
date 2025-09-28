package e2e_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/cmd"
)

func TestStackInfoSmoke(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "stack", "info")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "- main\n")
	require.Contains(t, stdout, "  - feature\n")
	require.Contains(t, stdout, "    - feature-child *\n")
}

func runStacky(t *testing.T, repoDir, homeDir string, stdin *bytes.Reader, args ...string) (string, string, error) {
	t.Helper()

	root := cmd.NewRootCommand()
	if stdin == nil {
		stdin = bytes.NewReader(nil)
	}
	root.SetIn(stdin)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)

	prevHome := os.Getenv("HOME")
	require.NoError(t, os.Setenv("HOME", homeDir))
	defer func() {
		if prevHome == "" {
			require.NoError(t, os.Unsetenv("HOME"))
			return
		}
		require.NoError(t, os.Setenv("HOME", prevHome))
	}()

	prevWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() {
		require.NoError(t, os.Chdir(prevWD))
	}()

	execErr := root.ExecuteContext(context.Background())

	return stdout.String(), stderr.String(), execErr
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "e2e@example.com")
	runGit(t, dir, "config", "user.name", "E2E Tester")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	runGit(t, dir, "config", "core.fsmonitor", "false")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644))
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")

	return dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	return string(out)
}
