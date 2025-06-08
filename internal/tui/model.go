package tui

import (
	"gobox/internal/core"
	"gobox/internal/state"
	"gobox/pkg/task"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	"time"
)

// TaskItem represents a task for the list.
type TaskItem struct {
	line string // single-line display: description + timebox
	task task.Task
}

func (t TaskItem) Title() string       { return t.line }
func (t TaskItem) Description() string { return "" }
func (t TaskItem) FilterValue() string { return t.line }

// model is the Bubbletea model for the TUI.
type model struct {
	list          list.Model
	quitting      bool
	timerActive   bool
	timer         time.Duration
	timerTotal    time.Duration
	timerTask     TaskItem
	timerDone     bool
	sessionRunner interface{} // session.SessionRunner, but avoid import cycle
	sessionState  *state.TimeBoxState
	gitWatcher    interface{} // gitwatcher.GitWatcher, but avoid import cycle
	commits       []string
	commitTable   table.Model
	height        int // Track terminal height for dynamic resizing
	width         int // Track terminal width for dynamic resizing

	// State file support
	stateMgr core.StateStore
	states   []state.TimeBoxState
}

// initialModel creates the initial TUI model.
func initialModel(tasks []TaskItem, markdownFile string, height int, stateMgr core.StateStore, states []state.TimeBoxState) model {
	items := make([]list.Item, len(tasks))
	for i, t := range tasks {
		items[i] = t
	}
	// Use a default height and width if not set yet
	listHeight := max(height-12, 5)
	defaultWidth := 80
	listDelegate := list.NewDefaultDelegate()
	listDelegate.ShowDescription = false
	l := list.New(items, listDelegate, defaultWidth, listHeight)
	// Remove default quit keys so we handle q/ctrl+c ourselves
	l.Title = markdownFile // Store the file path in the title for reloads
	// Initialize the commit table with a single column for commit messages
	columns := []table.Column{
		{Title: "Commit", Width: defaultWidth - 4},
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(false),
		table.WithHeight(10),
	)
	
	m := model{
		list:        l,
		height:      height,
		width:       defaultWidth,
		stateMgr:    stateMgr,
		states:      states,
		commitTable: t,
		commits:     []string{},
	}
	return m
}

// max returns the maximum of two ints.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}