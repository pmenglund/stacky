package stackgraph

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pmenglund/stacky/internal/gitstore"
)

// Branch represents a branch within a stack and its relationships.
type Branch struct {
	Name         string
	Parent       *Branch
	ParentCommit string
	Commit       string
	Children     []*Branch
}

// Graph holds all branches grouped by their stack roots.
type Graph struct {
	branches map[string]*Branch
	bottoms  []*Branch
}

// Load constructs a stack graph from the repository.
func Load(ctx context.Context, repo *gitstore.Repository) (*Graph, error) {
	names, err := repo.ListBranches(ctx)
	if err != nil {
		return nil, fmt.Errorf("stackgraph: list branches: %w", err)
	}

	// deterministically iterate when linking children for stable tests
	sort.Strings(names)

	type record struct {
		branch     *Branch
		parentName string
	}

	records := make(map[string]*record, len(names))

	for _, name := range names {
		commit, err := repo.ReadRef(ctx, fmt.Sprintf("refs/heads/%s", name))
		if err != nil {
			return nil, fmt.Errorf("stackgraph: read branch commit for %s: %w", name, err)
		}
		if commit == "" {
			return nil, fmt.Errorf("stackgraph: branch %s has no commit", name)
		}

		parentMerge, err := repo.BranchMergeTarget(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("stackgraph: read merge target for %s: %w", name, err)
		}

		parentName := deriveParentName(name, parentMerge)

		parentCommit, err := repo.ReadRef(ctx, fmt.Sprintf("refs/stack-parent/%s", name))
		if err != nil {
			return nil, fmt.Errorf("stackgraph: read parent commit for %s: %w", name, err)
		}

		records[name] = &record{
			branch: &Branch{
				Name:         name,
				ParentCommit: parentCommit,
				Commit:       commit,
			},
			parentName: parentName,
		}
	}

	g := &Graph{branches: make(map[string]*Branch, len(records))}

	for name, rec := range records {
		g.branches[name] = rec.branch
	}

	for name, rec := range records {
		if rec.parentName == "" {
			g.bottoms = append(g.bottoms, rec.branch)
			continue
		}

		parentRec, ok := records[rec.parentName]
		if !ok {
			return nil, fmt.Errorf("stackgraph: parent %s of branch %s not found", rec.parentName, name)
		}
		rec.branch.Parent = parentRec.branch
		parentRec.branch.Children = append(parentRec.branch.Children, rec.branch)
	}

	return g, nil
}

func deriveParentName(branch, merge string) string {
	merge = strings.TrimSpace(merge)
	if merge == "" {
		return ""
	}
	merge = strings.TrimPrefix(merge, "refs/heads/")
	if merge == branch {
		return ""
	}
	return merge
}

// Branch retrieves a branch by name.
func (g *Graph) Branch(name string) (*Branch, bool) {
	b, ok := g.branches[name]
	return b, ok
}

// Bottoms returns the stack roots.
func (g *Graph) Bottoms() []*Branch {
	return append([]*Branch(nil), g.bottoms...)
}

// Upstack returns the branch and all descendants in depth-first order.
func (g *Graph) Upstack(name string) ([]*Branch, error) {
	node, ok := g.branches[name]
	if !ok {
		return nil, fmt.Errorf("stackgraph: branch %s not found", name)
	}
	var result []*Branch
	var walk func(*Branch)
	walk = func(b *Branch) {
		result = append(result, b)
		sort.Slice(b.Children, func(i, j int) bool {
			return b.Children[i].Name < b.Children[j].Name
		})
		for _, child := range b.Children {
			walk(child)
		}
	}
	walk(node)
	return result, nil
}

// Downstack returns the chain of ancestors from the branch down to the stack bottom.
func (g *Graph) Downstack(name string) ([]*Branch, error) {
	node, ok := g.branches[name]
	if !ok {
		return nil, fmt.Errorf("stackgraph: branch %s not found", name)
	}
	var result []*Branch
	for b := node; b != nil; b = b.Parent {
		result = append(result, b)
	}
	return result, nil
}

// Forest returns all stack bottoms, suitable for tree rendering.
func (g *Graph) Forest() []*Branch {
	return g.Bottoms()
}
