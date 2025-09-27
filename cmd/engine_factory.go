package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmenglund/stacky/internal/engine"
)

func newEngine(ctx context.Context) (*engine.Engine, error) {
	opts, ok := ctx.Value(engineOptionsKey{}).(engine.Options)
	if !ok {
		opts = engine.Options{}
	}

	if opts.RepoPath == "" {
		opts.RepoPath = "."
	}

	if opts.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("determine home directory: %w", err)
		}
		opts.HomeDir = home
	}

	if opts.StatePath == "" && opts.HomeDir != "" {
		opts.StatePath = filepath.Join(opts.HomeDir, ".stacky.state")
	}

	return engine.New(ctx, opts)
}

type engineOptionsKey struct{}

func withEngineOptions(ctx context.Context, opts engine.Options) context.Context {
	return context.WithValue(ctx, engineOptionsKey{}, opts)
}
