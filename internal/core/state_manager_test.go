package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gobox/internal/state"
)

func sampleStates() []state.TimeBoxState {
	now := time.Now().Truncate(time.Second)
	later := now.Add(1 * time.Hour)
	return []state.TimeBoxState{
		{
			TaskHash: "hash1",
			Segments: []state.TimeSegment{
				{Start: now, End: &later},
			},
		},
		{
			TaskHash: "hash2",
			Segments: []state.TimeSegment{
				{Start: now, End: nil},
			},
		},
	}
}

func TestInMemoryStateStore_Basic(t *testing.T) {
	store := NewInMemoryStateStore()
	states := sampleStates()

	// Save and Load
	if err := store.Save(states); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !reflect.DeepEqual(states, loaded) {
		t.Errorf("Loaded state does not match saved state.\nGot:  %+v\nWant: %+v", loaded, states)
	}

	// RemoveTaskState
	remaining := store.RemoveTaskState(loaded, "hash1")
	if len(remaining) != 1 || remaining[0].TaskHash != "hash2" {
		t.Errorf("RemoveTaskState did not remove the correct task: %+v", remaining)
	}
}

func TestFileStateStore_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")
	store := NewFileStateStore(stateFile)
	states := sampleStates()

	// Save and Load
	if err := store.Save(states); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !reflect.DeepEqual(states, loaded) {
		t.Errorf("Loaded state does not match saved state.\nGot:  %+v\nWant: %+v", loaded, states)
	}

	// RemoveTaskState
	remaining := store.RemoveTaskState(loaded, "hash2")
	if len(remaining) != 1 || remaining[0].TaskHash != "hash1" {
		t.Errorf("RemoveTaskState did not remove the correct task: %+v", remaining)
	}

	// Save and Load after removal
	if err := store.Save(remaining); err != nil {
		t.Fatalf("Save after removal failed: %v", err)
	}
	loaded2, err := store.Load()
	if err != nil {
		t.Fatalf("Load after removal failed: %v", err)
	}
	if !reflect.DeepEqual(remaining, loaded2) {
		t.Errorf("Loaded state after removal does not match.\nGot:  %+v\nWant: %+v", loaded2, remaining)
	}

	// File should exist
	if _, err := os.Stat(stateFile); err != nil {
		t.Errorf("Expected state file to exist, but got error: %v", err)
	}
}
