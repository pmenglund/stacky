package ui

import (
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/pmenglund/stacky/internal/stackgraph"
)

// ColorMode mirrors the CLI flag controlling ANSI colour output.
type ColorMode string

const (
	// ColorModeAlways forces ANSI styling even when stdout is not a TTY.
	ColorModeAlways ColorMode = "always"
	// ColorModeAuto enables styling when the caller decides it is appropriate.
	ColorModeAuto ColorMode = "auto"
	// ColorModeNever disables ANSI styling.
	ColorModeNever ColorMode = "never"
)

// Renderer styles stack data for terminal output.
type Renderer struct {
	gloss        *lipgloss.Renderer
	branchStyle  lipgloss.Style
	currentStyle lipgloss.Style
}

// ForestRenderOptions controls how RenderForest decorates branches.
type ForestRenderOptions struct {
	// CurrentBranch, when non-empty, is annotated with a "*" marker and
	// rendered using the currentStyle.
	CurrentBranch string
	// Annotations maps branch names to inline suffixes, e.g. PR numbers.
	Annotations map[string]string
}

// New constructs a Renderer using the provided colour mode.
func New(mode ColorMode) *Renderer {
	gloss := lipgloss.NewRenderer(io.Discard)

	if mode == ColorModeNever {
		gloss.SetColorProfile(termenv.Ascii)
	} else {
		gloss.SetColorProfile(termenv.TrueColor)
	}

	branchStyle := gloss.NewStyle()
	currentStyle := gloss.NewStyle()

	if mode != ColorModeNever {
		branchStyle = branchStyle.Foreground(lipgloss.Color("244"))
		currentStyle = currentStyle.Foreground(lipgloss.Color("10")).Bold(true)
	}

	return &Renderer{
		gloss:        gloss,
		branchStyle:  branchStyle,
		currentStyle: currentStyle,
	}
}

// RenderForest prints a forest of stack branches with deterministic ordering.
func (r *Renderer) RenderForest(forest []*stackgraph.Branch, opts ForestRenderOptions) string {
	if len(forest) == 0 {
		return ""
	}

	sorted := append([]*stackgraph.Branch(nil), forest...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	var b strings.Builder
	for _, root := range sorted {
		r.renderBranch(&b, root, 0, opts)
	}
	return b.String()
}

func (r *Renderer) renderBranch(b *strings.Builder, branch *stackgraph.Branch, depth int, opts ForestRenderOptions) {
	var style lipgloss.Style
	label := branch.Name
	annotation := ""
	if opts.Annotations != nil {
		if text, ok := opts.Annotations[branch.Name]; ok && text != "" {
			annotation = " " + text
		}
	}
	if branch.Name == opts.CurrentBranch {
		style = r.currentStyle
		label = label + " *"
	} else {
		style = r.branchStyle
	}

	rendered := style.Render(label + annotation)
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString("- ")
	b.WriteString(rendered)
	b.WriteString("\n")

	children := append([]*stackgraph.Branch(nil), branch.Children...)
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})

	for _, child := range children {
		r.renderBranch(b, child, depth+1, opts)
	}
}
