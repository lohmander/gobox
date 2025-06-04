package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTimeBoxState_IsActive(t *testing.T) {
	now := time.Now()
	later := now.Add(1 * time.Hour)

	tests := []struct {
		name     string
		segments []TimeSegment
		want     bool
	}{
		{
			name:     "no segments",
			segments: nil,
			want:     false,
		},
		{
			name: "all segments ended",
			segments: []TimeSegment{
				{Start: now, End: &later},
			},
			want: false,
		},
		{
			name: "last segment ongoing",
			segments: []TimeSegment{
				{Start: now, End: &later},
				{Start: later, End: nil},
			},
			want: true,
		},
		{
			name: "single ongoing segment",
			segments: []TimeSegment{
				{Start: now, End: nil},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := &TimeBoxState{Segments: tt.segments}
			if got := tb.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTimeBoxState_SaveToFile_and_LoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "state.json")

	now := time.Now().Truncate(time.Second)
	later := now.Add(1 * time.Hour).Truncate(time.Second)

	original := &TimeBoxState{
		TaskHash: "abc123",
		Segments: []TimeSegment{
			{Start: now, End: &later},
			{Start: later, End: nil},
		},
	}

	// Test SaveToFile
	if err := original.SaveToFile(filePath); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Test file exists
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("Expected file to exist after SaveToFile, got error: %v", err)
	}

	// Test LoadFromFile
	loaded, err := LoadFromFile(filePath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Compare loaded state to original
	if loaded.TaskHash != original.TaskHash {
		t.Errorf("Loaded TaskHash = %v, want %v", loaded.TaskHash, original.TaskHash)
	}
	if len(loaded.Segments) != len(original.Segments) {
		t.Fatalf("Loaded Segments len = %d, want %d", len(loaded.Segments), len(original.Segments))
	}
	for i := range loaded.Segments {
		if !loaded.Segments[i].Start.Equal(original.Segments[i].Start) {
			t.Errorf("Segment %d Start = %v, want %v", i, loaded.Segments[i].Start, original.Segments[i].Start)
		}
		if (loaded.Segments[i].End == nil) != (original.Segments[i].End == nil) {
			t.Errorf("Segment %d End nil mismatch: got %v, want %v", i, loaded.Segments[i].End, original.Segments[i].End)
		} else if loaded.Segments[i].End != nil && !loaded.Segments[i].End.Equal(*original.Segments[i].End) {
			t.Errorf("Segment %d End = %v, want %v", i, loaded.Segments[i].End, original.Segments[i].End)
		}
	}
}

func TestTimeBoxState_CreatedAt(t *testing.T) {
	now := time.Now()
	later := now.Add(1 * time.Hour)

	tests := []struct {
		name     string
		segments []TimeSegment
		want     time.Time
	}{
		{
			name:     "no segments",
			segments: nil,
			want:     time.Time{},
		},
		{
			name: "single segment",
			segments: []TimeSegment{
				{Start: now, End: nil},
			},
			want: now,
		},
		{
			name: "multiple segments",
			segments: []TimeSegment{
				{Start: now, End: &later},
				{Start: later, End: nil},
			},
			want: now,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := &TimeBoxState{Segments: tt.segments}
			if got := tb.CreatedAt(); !got.Equal(tt.want) {
				t.Errorf("CreatedAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTimeBoxState_UpdatedAt(t *testing.T) {
	now := time.Now()
	later := now.Add(1 * time.Hour)
	muchLater := later.Add(1 * time.Hour)

	tests := []struct {
		name     string
		segments []TimeSegment
		want     time.Time
	}{
		{
			name:     "no segments",
			segments: nil,
			want:     time.Time{},
		},
		{
			name: "single ended segment",
			segments: []TimeSegment{
				{Start: now, End: &later},
			},
			want: later,
		},
		{
			name: "single ongoing segment",
			segments: []TimeSegment{
				{Start: now, End: nil},
			},
			want: now,
		},
		{
			name: "multiple segments, last ended",
			segments: []TimeSegment{
				{Start: now, End: &later},
				{Start: later, End: &muchLater},
			},
			want: muchLater,
		},
		{
			name: "multiple segments, last ongoing",
			segments: []TimeSegment{
				{Start: now, End: &later},
				{Start: later, End: nil},
			},
			want: later,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := &TimeBoxState{Segments: tt.segments}
			got := tb.UpdatedAt()
			if !got.Equal(tt.want) {
				t.Errorf("UpdatedAt() = %v, want %v", got, tt.want)
			}
		})
	}
}
