package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/stackgraph"
)

func TestStackForestForBranchIncludesPathAndSubtree(t *testing.T) {
	main := &stackgraph.Branch{Name: "main"}
	feature := &stackgraph.Branch{Name: "feature", Parent: main}
	sub := &stackgraph.Branch{Name: "sub", Parent: feature}
	other := &stackgraph.Branch{Name: "other", Parent: main}

	main.Children = []*stackgraph.Branch{feature, other}
	feature.Children = []*stackgraph.Branch{sub}

	forest := stackForestForBranch(feature)
	require.Len(t, forest, 1)

	root := forest[0]
	require.Equal(t, "main", root.Name)
	require.Len(t, root.Children, 1)

	child := root.Children[0]
	require.Equal(t, "feature", child.Name)
	require.Len(t, child.Children, 1)
	require.Equal(t, "sub", child.Children[0].Name)

	require.NotSame(t, feature, child)
	require.NotNil(t, child.Parent)
	require.Equal(t, "main", child.Parent.Name)
}

func TestDownstackForestForBranchKeepsPathOnly(t *testing.T) {
	main := &stackgraph.Branch{Name: "main"}
	feature := &stackgraph.Branch{Name: "feature", Parent: main}
	sub := &stackgraph.Branch{Name: "sub", Parent: feature}
	other := &stackgraph.Branch{Name: "other", Parent: main}

	main.Children = []*stackgraph.Branch{feature, other}
	feature.Children = []*stackgraph.Branch{sub}

	forest := downstackForestForBranch(feature)
	require.Len(t, forest, 1)

	root := forest[0]
	require.Equal(t, "main", root.Name)
	require.Len(t, root.Children, 1)
	require.Equal(t, "feature", root.Children[0].Name)
	require.Empty(t, root.Children[0].Children)
}

func TestUpstackForestForBranchClonesSubtree(t *testing.T) {
	feature := &stackgraph.Branch{Name: "feature"}
	sub := &stackgraph.Branch{Name: "sub", Parent: feature}
	feature.Children = []*stackgraph.Branch{sub}

	forest := upstackForestForBranch(feature)
	require.Len(t, forest, 1)
	root := forest[0]

	require.Equal(t, "feature", root.Name)
	require.Len(t, root.Children, 1)
	require.Equal(t, "sub", root.Children[0].Name)
	require.Nil(t, root.Parent)
	require.NotSame(t, feature, root)
}
