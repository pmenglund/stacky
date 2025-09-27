package stackgraph_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/gitstore"
	"github.com/pmenglund/stacky/internal/stackgraph"
)

func TestLoadBuildsGraph(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	runGit(t, repoDir, "config", "user.email", "graph@example.com")
	runGit(t, repoDir, "config", "user.name", "Graph Tester")
	runGit(t, repoDir, "config", "commit.gpgsign", "false")

	repo, err := gitstore.Open(ctx, repoDir)
	require.NoError(t, err)

	mainCommit := gitRevParse(t, repoDir, "main")

	// feature branch stacked on main
	runGit(t, repoDir, "checkout", "-b", "feature")
	writeFile(t, repoDir, "feature.txt", "feature work")
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature commit")
	runGit(t, repoDir, "config", "branch.feature.merge", "refs/heads/main")
	runGit(t, repoDir, "config", "branch.feature.remote", ".")
	runGit(t, repoDir, "update-ref", "refs/stack-parent/feature", mainCommit)

	featureCommit := gitRevParse(t, repoDir, "feature")

	// subfeature stacked on feature
	runGit(t, repoDir, "checkout", "-b", "subfeature")
	writeFile(t, repoDir, "subfeature.txt", "subfeature work")
	runGit(t, repoDir, "add", "subfeature.txt")
	runGit(t, repoDir, "commit", "-m", "subfeature commit")
	runGit(t, repoDir, "config", "branch.subfeature.merge", "refs/heads/feature")
	runGit(t, repoDir, "config", "branch.subfeature.remote", ".")
	runGit(t, repoDir, "update-ref", "refs/stack-parent/subfeature", featureCommit)

	g, err := stackgraph.Load(ctx, repo)
	require.NoError(t, err)

	bottoms := g.Bottoms()
	require.Len(t, bottoms, 1)
	require.Equal(t, "main", bottoms[0].Name)

	feature, ok := g.Branch("feature")
	require.True(t, ok, "feature branch missing")
	require.NotNil(t, feature.Parent)
	require.Equal(t, "main", feature.Parent.Name)
	require.Equal(t, mainCommit, feature.ParentCommit)

	subfeature, ok := g.Branch("subfeature")
	require.True(t, ok, "subfeature branch missing")
	require.NotNil(t, subfeature.Parent)
	require.Equal(t, "feature", subfeature.Parent.Name)

	upstack, err := g.Upstack("feature")
	require.NoError(t, err)
	upNames := names(upstack)
	require.Equal(t, []string{"feature", "subfeature"}, upNames)

	downstack, err := g.Downstack("subfeature")
	require.NoError(t, err)
	downNames := names(downstack)
	require.Equal(t, []string{"subfeature", "feature", "main"}, downNames)
}

func TestLoadErrorsOnMissingParentBranch(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	runGit(t, repoDir, "config", "user.email", "graph@example.com")
	runGit(t, repoDir, "config", "user.name", "Graph Tester")
	runGit(t, repoDir, "config", "commit.gpgsign", "false")

	repo, err := gitstore.Open(ctx, repoDir)
	require.NoError(t, err)

	runGit(t, repoDir, "checkout", "-b", "feature")
	runGit(t, repoDir, "config", "branch.feature.merge", "refs/heads/missing")

	_, err = stackgraph.Load(ctx, repo)
	require.Error(t, err)
}

func names(branches []*stackgraph.Branch) []string {
	out := make([]string, len(branches))
	for i, b := range branches {
		out[i] = b.Name
	}
	return out
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "graph@example.com")
	runGit(t, dir, "config", "user.name", "Graph Tester")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	writeFile(t, dir, "README.md", "root")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial commit")

	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

func gitRevParse(t *testing.T, dir, rev string) string {
	t.Helper()
	out := runGit(t, dir, "rev-parse", rev)
	return strings.TrimSpace(out)
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
