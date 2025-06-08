package state

import (
	"encoding/json"
	"os"
	"time"
)

// TimeBoxState represents the state of a timeboxed task, including its unique identifier
// and the list of time segments (work intervals) associated with it.
// This struct is designed to be serializable for persistence between sessions.
type TimeBoxState struct {
	TaskHash  string        `json:"task_hash"`  // Unique hash of the task
	Segments  []TimeSegment `json:"segments"`   // List of time segments
	Completed bool          `json:"completed"`  // Whether the task is completed
}

// TimeSegment represents a single uninterrupted interval of work within a timebox.
// Start marks the beginning of the segment, and End is nil if the segment is ongoing.
type TimeSegment struct {
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end"`
}

// IsActive reports whether the timebox is currently active.
// A timebox is considered active if its last segment has no End time (i.e., work is ongoing).
func (t *TimeBoxState) IsActive() bool {
	if len(t.Segments) == 0 {
		return false
	}
	last := t.Segments[len(t.Segments)-1]
	return last.End == nil
}

// CreatedAt returns the start time of the first segment, representing when the timebox was started.
// If there are no segments, it returns the zero value of time.Time.
func (t *TimeBoxState) CreatedAt() time.Time {
	if len(t.Segments) == 0 {
		return time.Time{}
	}
	return t.Segments[0].Start
}

// UpdatedAt returns the most recent update time for the timebox.
// This is the End time of the last segment if it exists, otherwise the Start time of the last segment.
// If there are no segments, it returns the zero value of time.Time.
func (t *TimeBoxState) UpdatedAt() time.Time {
	if len(t.Segments) == 0 {
		return time.Time{}
	}
	last := t.Segments[len(t.Segments)-1]
	if last.End != nil {
		return *last.End
	}
	return last.Start
}

// SaveToFile serializes the TimeBoxState to a file as JSON.
func (t *TimeBoxState) SaveToFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(t)
}

// LoadFromFile deserializes a TimeBoxState from a JSON file.
func LoadFromFile(path string) (*TimeBoxState, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var t TimeBoxState
	dec := json.NewDecoder(f)
	if err := dec.Decode(&t); err != nil {
		return nil, err
	}
	return &t, nil
}
