package ui_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/stackgraph"
	"github.com/pmenglund/stacky/internal/ui"
)

func TestRenderForestWithoutColor(t *testing.T) {
	renderer := ui.New(ui.ColorModeNever)

	main := &stackgraph.Branch{Name: "main"}
	feature := &stackgraph.Branch{Name: "feature", Parent: main}
	bugfix := &stackgraph.Branch{Name: "bugfix", Parent: main}
	main.Children = []*stackgraph.Branch{feature, bugfix}

	out := renderer.RenderForest([]*stackgraph.Branch{main}, ui.ForestRenderOptions{CurrentBranch: "feature"})

	expected := "- main\n  - bugfix\n  - feature *\n"
	require.Equal(t, expected, out)
}

func TestRenderForestWithColor(t *testing.T) {
	renderer := ui.New(ui.ColorModeAlways)

	branch := &stackgraph.Branch{Name: "main"}

	out := renderer.RenderForest([]*stackgraph.Branch{branch}, ui.ForestRenderOptions{CurrentBranch: "main"})

	require.Contains(t, out, "main *")
	require.Contains(t, out, "\x1b[")
}

func TestRenderForestEmpty(t *testing.T) {
	renderer := ui.New(ui.ColorModeNever)
	out := renderer.RenderForest(nil, ui.ForestRenderOptions{})
	require.Empty(t, out)
}

func TestRenderForestWithAnnotations(t *testing.T) {
	renderer := ui.New(ui.ColorModeNever)

	main := &stackgraph.Branch{Name: "main"}
	feature := &stackgraph.Branch{Name: "feature", Parent: main}
	main.Children = []*stackgraph.Branch{feature}

	annotations := map[string]string{"feature": "(#7)"}
	out := renderer.RenderForest([]*stackgraph.Branch{main}, ui.ForestRenderOptions{CurrentBranch: "feature", Annotations: annotations})

	require.Contains(t, out, "feature * (#7)")
}
