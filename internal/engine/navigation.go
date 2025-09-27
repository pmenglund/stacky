package engine

import (
	"context"
	"fmt"
	"sort"
)

// UpBranch returns the first child branch in lexical order.
func (e *Engine) UpBranch(ctx context.Context, current string) (string, error) {
	branch, err := e.Branch(ctx, current)
	if err != nil {
		return "", err
	}
	if len(branch.Children) == 0 {
		return "", fmt.Errorf("engine: branch %s has no children", current)
	}
	sort.Slice(branch.Children, func(i, j int) bool {
		return branch.Children[i].Name < branch.Children[j].Name
	})
	return branch.Children[0].Name, nil
}

// DownBranch returns the parent branch.
func (e *Engine) DownBranch(ctx context.Context, current string) (string, error) {
	branch, err := e.Branch(ctx, current)
	if err != nil {
		return "", err
	}
	if branch.Parent == nil {
		return "", fmt.Errorf("engine: branch %s is already at stack bottom", current)
	}
	return branch.Parent.Name, nil
}
