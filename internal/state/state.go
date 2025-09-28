package state

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// State mirrors the JSON structure written by the legacy Python tool.
type State struct {
	Branch    string          `json:"branch"`
	Sync      []string        `json:"sync,omitempty"`
	Fold      *FoldState      `json:"fold,omitempty"`
	MergeFold *MergeFoldState `json:"merge_fold,omitempty"`
}

// FoldState captures progress for cherry-pick based fold operations.
type FoldState struct {
	FoldBranch   string   `json:"fold_branch"`
	ParentBranch string   `json:"parent_branch"`
	Commits      []string `json:"commits,omitempty"`
	Children     []string `json:"children,omitempty"`
	AllowEmpty   bool     `json:"allow_empty,omitempty"`
}

// MergeFoldState captures progress for merge based fold operations.
type MergeFoldState struct {
	FoldBranch   string   `json:"fold_branch"`
	ParentBranch string   `json:"parent_branch"`
	Children     []string `json:"children,omitempty"`
}

// Load reads state from disk. If the file does not exist an empty State and nil
// error are returned.
func Load(path string) (State, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("state: open %s: %w", path, err)
	}
	defer f.Close()

	bytes, err := io.ReadAll(f)
	if err != nil {
		return State{}, fmt.Errorf("state: read %s: %w", path, err)
	}

	if len(bytes) == 0 {
		return State{}, nil
	}

	var st State
	if err := json.Unmarshal(bytes, &st); err != nil {
		return State{}, fmt.Errorf("state: decode %s: %w", path, err)
	}

	return st, nil
}

// Save writes state atomically using a temporary file followed by rename.
func Save(path string, st State) error {
	tmp := path + ".tmp"

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("state: encode: %w", err)
	}

	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("state: write temp file %s: %w", tmp, err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("state: rename %s to %s: %w", tmp, path, err)
	}

	return nil
}

// Clear removes the state file along with any temporary file residue.
func Clear(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("state: remove %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("state: remove %s: %w", tmp, err)
	}
	return nil
}
