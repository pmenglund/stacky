package gitstore_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/gitstore"
)

func TestOpenAndBasicOperations(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	repo, err := gitstore.Open(ctx, repoDir)
	require.NoError(t, err)

	branch, err := repo.CurrentBranch(ctx)
	require.NoError(t, err)
	require.Equal(t, "main", branch)

	branches, err := repo.ListBranches(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"main"}, branches)

	head, err := repo.ReadRef(ctx, "refs/heads/main")
	require.NoError(t, err)
	require.NotEmpty(t, head)

	require.NoError(t, repo.CreateBranch(ctx, "feature"))

	require.NoError(t, repo.Checkout(ctx, "feature"))

	featureHead, err := repo.ReadRef(ctx, "refs/heads/feature")
	require.NoError(t, err)
	require.NotEmpty(t, featureHead)

	status, err := repo.Run(ctx, "status", "--short")
	require.NoError(t, err)
	require.Equal(t, "", strings.TrimSpace(status))

	currentHead, err := repo.HeadCommit(ctx)
	require.NoError(t, err)
	require.Equal(t, featureHead, currentHead)

	require.NoError(t, repo.WriteRef(ctx, "refs/stack-parent/feature", head, ""))

	parent, err := repo.ReadRef(ctx, "refs/stack-parent/feature")
	require.NoError(t, err)
	require.Equal(t, head, parent)
}

func TestReadRefMissingReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	repo, err := gitstore.Open(ctx, repoDir)
	require.NoError(t, err)

	value, err := repo.ReadRef(ctx, "refs/stack-parent/missing")
	require.NoError(t, err)
	require.Empty(t, value)
}

func TestInitCreatesRepository(t *testing.T) {
	ctx := context.Background()
	repoPath := filepath.Join(t.TempDir(), "repo")

	repo, err := gitstore.Init(ctx, repoPath)
	require.NoError(t, err)
	require.NotNil(t, repo)

	abs, err := filepath.Abs(repoPath)
	require.NoError(t, err)
	root := repo.Root()
	rootReal, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	absReal, err := filepath.EvalSymlinks(abs)
	require.NoError(t, err)
	require.Equal(t, absReal, rootReal)

	info, err := os.Stat(filepath.Join(abs, ".git"))
	require.NoError(t, err)
	require.True(t, info.IsDir(), "expected .git to be a directory")

	_, err = repo.Run(ctx, "config", "user.email", "test@example.com")
	require.NoError(t, err)
	_, err = repo.Run(ctx, "config", "user.name", "Test User")
	require.NoError(t, err)
	_, err = repo.Run(ctx, "config", "commit.gpgsign", "false")
	require.NoError(t, err)

	readme := filepath.Join(abs, "README.md")
	require.NoError(t, os.WriteFile(readme, []byte("hello"), 0o644))

	_, err = repo.Run(ctx, "add", "README.md")
	require.NoError(t, err)
	_, err = repo.Run(ctx, "commit", "-m", "initial commit")
	require.NoError(t, err)

	head, err := repo.HeadCommit(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, head)
}

func TestOpenFailsOutsideRepo(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	_, err := gitstore.Open(ctx, dir)
	require.Error(t, err)
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644))

	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial commit")

	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
}
