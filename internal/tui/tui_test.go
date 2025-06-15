package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"gobox/internal/core"
	"gobox/internal/state"
	"gobox/pkg/task"

	tea "github.com/charmbracelet/bubbletea"
)

// TestCompletionScreenConfirmTriggersMarkdownUpdate tests that pressing enter or space on the completion screen
// marks the task as completed and updates the markdown file accordingly.
func TestCompletionScreenConfirmTriggersMarkdownUpdate(t *testing.T) {
	markdownContent := "- [ ] Sample Test Task for `some work` @1m\n"
	tmpFile, err := os.CreateTemp("", "gobox_tui_test_*.md")
	if err != nil {
		t.Fatalf("failed to create temp markdown file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(markdownContent); err != nil {
		t.Fatalf("failed to write markdown content: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp markdown file: %v", err)
	}

	stateMgr := core.NewInMemoryStateStore()
	states, _ := stateMgr.Load()

	// Initialize TUI model using InitialModel with the temp markdown file
	tasks := []TaskItem{
		{
			RawLine: "Sample Test Task for `some work` @1m",
			Task: task.Task{
				Description: "Sample Test Task for some work",
				TimeBox:     "@1m",
				IsChecked:   false,
			},
			Width: 80,
		},
	}

	model := InitialModel(tasks, tmpFile.Name(), 24, stateMgr, states)

	// Simulate starting the timer by manually setting model state
	model.ActiveView = ViewTimerDone
	model.TimerTask = tasks[0]

	// Simulate session state for completion with one segment with start and end time
	now := time.Now()
	sessState := state.TimeBoxState{
		TaskHash: tasks[0].Task.Hash(),
		Segments: []state.TimeSegment{
			{Start: now.Add(-1 * time.Minute), End: &now},
		},
	}
	model.SessionState = &sessState
	model.States = append(model.States, sessState)

	// Simulate pressing Enter key in ViewTimerDone, which should trigger markdown update
	keyEnter := simulateKeyMsg("enter")

	model, _ = HandleKeyMsg(model, keyEnter)

	// Read back updated markdown file contents
	updatedContent, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to read updated markdown file: %v", err)
	}
	updatedStr := string(updatedContent)

	if !strings.Contains(updatedStr, "[x] Sample Test Task @1m") {
		t.Errorf("Task was not marked as completed in markdown:\n%s", updatedStr)
	}
	if !strings.Contains(updatedStr, "⏱️") {
		t.Errorf("Duration annotation missing from markdown:\n%s", updatedStr)
	}
}

// simulateKeyMsg creates a tea.KeyMsg for a given string key
func simulateKeyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(key),
	}
}
