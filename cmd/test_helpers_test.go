package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "cmd@example.com")
	runGit(t, dir, "config", "user.name", "Cmd Tester")
	runGit(t, dir, "config", "commit.gpgsign", "false")

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

func withWorkingDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	defer func() {
		require.NoError(t, os.Chdir(wd))
	}()
	fn()
}
