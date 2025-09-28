package tui

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectByIndex(t *testing.T) {
	in := bytes.NewBufferString("2\n")
	out := &bytes.Buffer{}

	choice, err := Select(in, out, []string{"feature", "bugfix"}, SelectOptions{Prompt: "Select branch", Highlight: "feature"})
	require.NoError(t, err)
	require.Equal(t, "bugfix", choice)

	contents := out.String()
	require.Contains(t, contents, "Select branch:")
	require.Contains(t, contents, "1) feature *")
	require.Contains(t, contents, "2) bugfix")
}

func TestSelectByName(t *testing.T) {
	in := bytes.NewBufferString("feature\n")
	out := &bytes.Buffer{}

	choice, err := Select(in, out, []string{"feature", "bugfix"}, SelectOptions{Prompt: "Choose"})
	require.NoError(t, err)
	require.Equal(t, "feature", choice)
}

func TestSelectInvalidThenValid(t *testing.T) {
	in := bytes.NewBufferString("unknown\n1\n")
	out := &bytes.Buffer{}

	choice, err := Select(in, out, []string{"alpha", "beta"}, SelectOptions{Prompt: "Pick"})
	require.NoError(t, err)
	require.Equal(t, "alpha", choice)
	require.Contains(t, out.String(), "Invalid selection \"unknown\"")
}

func TestSelectNoOptions(t *testing.T) {
	in := bytes.NewBuffer(nil)
	out := &bytes.Buffer{}

	_, err := Select(in, out, nil, SelectOptions{Prompt: "Pick"})
	require.Error(t, err)
}

func TestIsTerminalPipe(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})

	require.False(t, IsTerminal(r))
}

func TestIsTerminalNonFile(t *testing.T) {
	in := bytes.NewBuffer(nil)
	require.True(t, IsTerminal(in))
}
