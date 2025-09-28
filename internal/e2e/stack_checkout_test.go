package e2e_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStackCheckoutInteractiveSelection(t *testing.T) {
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

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "checkout", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	input := bytes.NewReader([]byte("feature-child\n"))
	stdout, stderr, err = runStacky(t, repoDir, homeDir, input, "--color=never", "stack", "checkout")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Select branch in stack:")
	require.Contains(t, stdout, "  1) feature-child")
	require.Contains(t, stdout, "  2) main")
	require.Contains(t, stdout, "> ")

	head := strings.TrimSpace(runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"))
	require.Equal(t, "feature-child", head)
}
