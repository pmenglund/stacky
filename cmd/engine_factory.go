package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmenglund/stacky/internal/engine"
)

func newEngine(ctx context.Context) (*engine.Engine, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determine home directory: %w", err)
	}

	opts := engine.Options{
		RepoPath:  ".",
		HomeDir:   home,
		StatePath: filepath.Join(home, ".stacky.state"),
	}

	return engine.New(ctx, opts)
}
