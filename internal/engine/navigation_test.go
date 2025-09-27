package engine_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/engine"
)

func TestUpAndDownBranch(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir()})
	require.NoError(t, err)

	up, err := eng.UpBranch(ctx, "main")
	require.NoError(t, err)
	require.Equal(t, "feature", up)

	down, err := eng.DownBranch(ctx, "feature")
	require.NoError(t, err)
	require.Equal(t, "main", down)
}
