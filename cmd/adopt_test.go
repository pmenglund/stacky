package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdoptCommandWiresBranch(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	runGit(t, repoDir, "checkout", "-b", "topic")
	runGit(t, repoDir, "checkout", "main")

	cmd := newAdoptCmd()
	cmd.SetArgs([]string{"topic"})
	cmd.SetContext(context.Background())

	withWorkingDir(t, repoDir, func() {
		require.NoError(t, cmd.Execute())
	})

	merge := strings.TrimSpace(runGit(t, repoDir, "config", "branch.topic.merge"))
	require.Equal(t, "refs/heads/main", merge)
}
