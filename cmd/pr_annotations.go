package cmd

import (
	"context"
	"fmt"

	"github.com/pmenglund/stacky/internal/engine"
)

func loadPRAnnotations(ctx context.Context, eng *engine.Engine, branches []string) (map[string]string, error) {
	if len(branches) == 0 {
		return nil, nil
	}

	prs, err := eng.OpenPullRequests(ctx)
	if err != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return nil, nil
	}

	annotations := make(map[string]string)
	seen := make(map[string]struct{}, len(branches))
	for _, name := range branches {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if pr, ok := prs[name]; ok {
			annotations[name] = fmt.Sprintf("(#%d)", pr.Number)
		}
	}
	if len(annotations) == 0 {
		return nil, nil
	}
	return annotations, nil
}
