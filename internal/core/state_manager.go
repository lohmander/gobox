package core

import (
	"encoding/json"
	"os"
	"sync"

	"gobox/internal/state"
)

// StateStore abstracts state persistence for testability.
type StateStore interface {
	Load() ([]state.TimeBoxState, error)
	Save([]state.TimeBoxState) error
	RemoveTaskState([]state.TimeBoxState, string) []state.TimeBoxState
}

// FileStateStore implements StateStore using a file.
type FileStateStore struct {
	File string
}

func NewFileStateStore(file string) *FileStateStore {
	return &FileStateStore{File: file}
}

func (fs *FileStateStore) Load() ([]state.TimeBoxState, error) {
	var states []state.TimeBoxState
	f, err := os.Open(fs.File)
	if err == nil {
		defer f.Close()
		dec := json.NewDecoder(f)
		if err := dec.Decode(&states); err != nil && err.Error() != "EOF" {
			return nil, err
		}
	}
	return states, nil
}

func (fs *FileStateStore) Save(states []state.TimeBoxState) error {
	f, err := os.Create(fs.File)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(states)
}

func (fs *FileStateStore) RemoveTaskState(states []state.TimeBoxState, taskHash string) []state.TimeBoxState {
	var newStates []state.TimeBoxState
	for _, s := range states {
		if s.TaskHash != taskHash {
			newStates = append(newStates, s)
		}
	}
	return newStates
}

// InMemoryStateStore implements StateStore for testing (no disk I/O).
type InMemoryStateStore struct {
	mu     sync.Mutex
	states []state.TimeBoxState
}

func NewInMemoryStateStore() *InMemoryStateStore {
	return &InMemoryStateStore{}
}

func (ms *InMemoryStateStore) Load() ([]state.TimeBoxState, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	// Return a copy to avoid mutation
	cpy := make([]state.TimeBoxState, len(ms.states))
	copy(cpy, ms.states)
	return cpy, nil
}

func (ms *InMemoryStateStore) Save(states []state.TimeBoxState) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	// Store a copy to avoid mutation
	cpy := make([]state.TimeBoxState, len(states))
	copy(cpy, states)
	ms.states = cpy
	return nil
}

func (ms *InMemoryStateStore) RemoveTaskState(states []state.TimeBoxState, taskHash string) []state.TimeBoxState {
	var newStates []state.TimeBoxState
	for _, s := range states {
		if s.TaskHash != taskHash {
			newStates = append(newStates, s)
		}
	}
	return newStates
}
