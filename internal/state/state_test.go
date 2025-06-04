package state

import (
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
