package tui

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"gobox/internal/parser"
	"gobox/internal/state"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbletea"
)

// dummyStateMgr is a testing stub for the core.StateStore interface.
type dummyStateMgr struct{}

func (d dummyStateMgr) Save(states []state.TimeBoxState) error {
	return nil
}
func (d dummyStateMgr) RemoveTaskState(states []state.TimeBoxState, taskHash string) []state.TimeBoxState {
	var newStates []state.TimeBoxState
	for _, s := range states {
		if s.TaskHash != taskHash {
			newStates = append(newStates, s)
		}
	}
	return newStates
}

func TestHandleSessionCompletedMsg_ReloadsTasks(t *testing.T) {
	// Create a temporary markdown file with two tasks.
	markdownContent := `
- [ ] Task A @10m
- [x] Task B @15m
`
	tmpFile, err := os.CreateTemp("", "test_tasks_*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(strings.TrimSpace(markdownContent))
	if err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Initialize the model using InitialModel with an empty tasks slice.
	// The list title will store the markdown file path.
	initialTasks := []list.Item{}
	height := 40
	states := []state.TimeBoxState{}
	sm := dummyStateMgr{}
	m := InitialModel(initialTasks, tmpFile.Name(), height, sm, states)

	// Set a non-nil SessionState to trigger the reload branch in handleSessionCompletedMsg.
	// For testing we don't care about its internal fields.
	m.SessionState = &state.TimeBoxState{
		TaskHash: "dummy",
		Segments: []state.TimeSegment{
			{Start: time.Now(), End: func() *time.Time { t := time.Now().Add(10 * time.Minute); return &t }()},
		},
	}

	// Call handleSessionCompletedMsg via Update.
	updatedModel, _ := handleSessionCompletedMsg(m, sessionCompletedMsg{})

	// Parse the markdown file independently to know expected tasks.
	parsedTasks, err := parser.ParseMarkdownFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to parse markdown file: %v", err)
	}

	// Extract the list items from the updated model.
	listItems := updatedModel.list.Items()

	if len(listItems) != len(parsedTasks) {
		t.Errorf("expected %d list items, got %d", len(parsedTasks), len(listItems))
	}

	// Further verify that the tasks in the list match the parsed tasks.
	var reloadedTasks []string
	for _, item := range listItems {
		// Assuming TaskItem.RawLine holds the task text.
		if ti, ok := item.(TaskItem); ok {
			reloadedTasks = append(reloadedTasks, ti.RawLine)
		}
	}

	var expectedTasks []string
	for _, pt := range parsedTasks {
		// Using the task's String() method for expected text.
		expectedTasks = append(expectedTasks, pt.String())
	}

	if !reflect.DeepEqual(reloadedTasks, expectedTasks) {
		t.Errorf("reloaded tasks do not match expected tasks.\nGot: %v\nExpected: %v", reloadedTasks, expectedTasks)
	}
}
