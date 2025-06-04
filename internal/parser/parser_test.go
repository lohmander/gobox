package parser_test

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"gobox/internal/parser"
	"gobox/pkg/task"
)

func TestParseMarkdownFile(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     []task.Task
		wantErr  bool
	}{
		{
			name:     "empty file",
			markdown: "",
			want:     []task.Task{},
			wantErr:  false,
		},
		{
			name:     "single unchecked task",
			markdown: "- [ ] Task 1 @1h",
			want: []task.Task{
				{
					Description: "Task 1",
					TimeBox:     "@1h",
					IsChecked:   false,
				},
			},
			wantErr: false,
		},
		{
			name:     "single checked task",
			markdown: "- [x] Task 2 @2h",
			want: []task.Task{
				{
					Description: "Task 2",
					TimeBox:     "@2h",
					IsChecked:   true,
				},
			},
			wantErr: false,
		},
		{
			name:     "multiple tasks",
			markdown: "- [ ] Task 1 @1h\n- [x] Task 2 @30m\n- [ ] Task 3",
			want: []task.Task{
				{
					Description: "Task 1",
					TimeBox:     "@1h",
					IsChecked:   false,
				},
				{
					Description: "Task 2",
					TimeBox:     "@30m",
					IsChecked:   true,
				},
				{
					Description: "Task 3",
					TimeBox:     "",
					IsChecked:   false,
				},
			},
			wantErr: false,
		},
		{
			name:     "regular list items",
			markdown: "- Task without checkbox",
			want:     []task.Task{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, err := createTempFileWithContent(tt.markdown)
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer tmpFile.Close()

			got, err := parser.ParseMarkdownFile(tmpFile.Name())
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMarkdownFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseMarkdownFile() got = %T %v, want %T %v", got, got, tt.want, tt.want)
			}
		})
	}
}

// Helper function to create a temporary file with the given content
func createTempFileWithContent(content string) (*os.File, error) {
	tmpFile, err := os.CreateTemp("", "test_markdown")
	if err != nil {
		return nil, err
	}
	_, err = tmpFile.WriteString(content)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, err
	}
	return tmpFile, nil
}

func TestParseTimeBox(t *testing.T) {
	tests := []struct {
		name         string
		timeBox      string
		wantDuration time.Duration
		wantEndTime  time.Time
		wantErr      bool
	}{
		// Add test cases for ParseTimeBox here
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDuration, gotEndTime, err := parser.ParseTimeBox(tt.timeBox)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTimeBox() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotDuration != tt.wantDuration {
				t.Errorf("ParseTimeBox() gotDuration = %v, wantDuration %v", gotDuration, tt.wantDuration)
			}
			// We can't directly compare time.Time values easily due to location,
			// so we'll compare the Unix timestamps if wantEndTime is not zero.
			if !tt.wantEndTime.IsZero() {
				if !gotEndTime.Equal(tt.wantEndTime) {
					t.Errorf("ParseTimeBox() gotEndTime = %v, wantEndTime %v", gotEndTime, tt.wantEndTime)
				}
			}
		})
	}
}

func TestUpdateMarkdown(t *testing.T) {
	original := "- [ ] Task 1 @1h\n- [ ] Task 2 @2h\n"
	tmpFile, err := createTempFileWithContent(original)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Parse tasks to get the one we want to update
	tasks, err := parser.ParseMarkdownFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("ParseMarkdownFile failed: %v", err)
	}
	if len(tasks) < 1 {
		t.Fatalf("expected at least one task")
	}

	// Update the first task: mark as checked and change description
	updated := tasks[0]
	updated.IsChecked = true
	updated.Description = "Task 1"
	updated.TimeBox = "@1h"

	// Simulate a duration
	start := time.Now()
	end := start.Add(1 * time.Hour)

	// Use totalDuration instead of start/end
	totalDuration := end.Sub(start)
	err = parser.UpdateMarkdown(tmpFile.Name(), updated, nil, totalDuration)
	if err != nil {
		t.Fatalf("UpdateMarkdown failed: %v", err)
	}

	// Read back the file and check the update
	updatedContent, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	updatedStr := string(updatedContent)
	if !strings.Contains(updatedStr, "[x] Task 1 @1h") {
		t.Errorf("updated task not found or incorrect: %q", updatedStr)
	}
	if !strings.Contains(updatedStr, "⏱️ 1h 0m 0s") {
		t.Errorf("duration not found or incorrect: %q", updatedStr)
	}
	if !strings.Contains(updatedStr, "- [ ] Task 2 @2h") {
		t.Errorf("other tasks should remain unchanged: %q", updatedStr)
	}
}
