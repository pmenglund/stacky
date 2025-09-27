package state_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/state"
)

func TestLoadMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	st, err := state.Load(filepath.Join(dir, "state.json"))
	require.NoError(t, err)
	require.True(t, statesEqual(st, state.State{}))
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	expected := state.State{
		Branch: "feature",
		Sync:   []string{"a", "b"},
		Fold: map[string]interface{}{
			"fold_branch": "feature",
		},
	}

	require.NoError(t, state.Save(path, expected))

	actual, err := state.Load(path)
	require.NoError(t, err)
	require.Truef(t, statesEqual(expected, actual), "states differ\nexpected: %#v\nactual: %#v", expected, actual)
}

func TestClearRemovesState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	require.NoError(t, os.WriteFile(path, []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(path+".tmp", []byte("{}"), 0o644))

	require.NoError(t, state.Clear(path))

	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(path + ".tmp")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func statesEqual(a, b state.State) bool {
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(aa) == string(bb)
}
