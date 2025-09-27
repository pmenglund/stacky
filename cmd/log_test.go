package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogCommandPrintsGitHistory(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	cmd := newLogCmd()
	cmd.SetContext(context.Background())
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})

	withWorkingDir(t, repoDir, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})

	require.Contains(t, out.String(), "init")
}
