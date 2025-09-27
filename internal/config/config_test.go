package config_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/config"
)

func TestLoadDefaultsWhenNoFiles(t *testing.T) {
	cfg, err := config.Load(context.Background(), "", "")
	require.NoError(t, err)
	require.Equal(t, config.Config{UseForcePush: true}, cfg)
}

func TestLoadMergesHomeAndRepoConfig(t *testing.T) {
	ctx := context.Background()

	home := t.TempDir()
	repo := t.TempDir()

	homeCfg := `[UI]
skip_confirm = true
change_to_main = true
share_ssh_session = true

[GIT]
use_force_push = false
`
	writeConfig(t, filepath.Join(home, ".stackyconfig"), homeCfg)

	repoCfg := `[UI]
change_to_main = false
compact_pr_display = true

[GIT]
use_merge = true
`
	writeConfig(t, filepath.Join(repo, ".stackyconfig"), repoCfg)

	cfg, err := config.Load(ctx, home, repo)
	require.NoError(t, err)

	require.Equal(t, config.Config{
		SkipConfirm:      true,
		ChangeToMain:     false,
		ChangeToAdopted:  false,
		ShareSSHSession:  true,
		UseMerge:         true,
		UseForcePush:     false,
		CompactPRDisplay: true,
	}, cfg)
}

func TestLoadReportsInvalidFiles(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	writeConfig(t, filepath.Join(home, ".stackyconfig"), `[UI]
skip_confirm = notabool
`)

	_, err := config.Load(ctx, home, "")
	require.Error(t, err)
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
