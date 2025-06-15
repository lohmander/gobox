package tui

import (
	"fmt"
	"gobox/internal/core"
	"gobox/internal/state"
	"gobox/pkg/task"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// TaskItem represents a task for the list.
type TaskItem struct {
	rawLine string // raw unwrapped line: description + timebox
	task    task.Task
	width   int // current width to wrap at
}

func (t *TaskItem) SetWidth(w int) {
	t.width = w
}

func (t TaskItem) Title() string {
	if t.width > 0 {
		return wrapText(t.rawLine, t.width)
	}
	return t.rawLine
}

func (t TaskItem) Description() string { return "" }
func (t TaskItem) FilterValue() string {
	if t.width > 0 {
		return wrapText(t.rawLine, t.width)
	}
	return t.rawLine
}

// ViewState determines which view is active in the TUI.
type ViewState int

const (
	ViewTaskList ViewState = iota
	ViewTimerActive
	ViewTimerDone
	ViewQuitting
)

// multilineDelegate wraps a list.DefaultDelegate and overrides Render to support multiline wrapped titles.
// It otherwise preserves default styling and behavior.
type multilineDelegate struct {
	list.DefaultDelegate

	titleStyle lipgloss.Style
	descStyle  lipgloss.Style
}

// Render renders a list item with multiline wrapped text for the title.
func (d *multilineDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ti, ok := item.(TaskItem)
	if !ok {
		d.DefaultDelegate.Render(w, m, index, item)
		return
	}

	title := ti.Title()
	lines := strings.Split(title, "\n")
	isSelected := index == m.Index()

	for i, line := range lines {
		if isSelected {
			fmt.Fprint(w, d.titleStyle.Render(line))
		} else {
			fmt.Fprint(w, line)
		}
		if i < len(lines)-1 {
			fmt.Fprint(w, "\n")
		}
	}

	desc := ti.Description()
	if desc != "" && d.ShowDescription {
		fmt.Fprint(w, "\n")
		if isSelected {
			fmt.Fprint(w, d.descStyle.Render(desc))
		} else {
			fmt.Fprint(w, desc)
		}
	}
}

type model struct {
	list          list.Model
	activeView    ViewState
	timer         time.Duration
	timerTotal    time.Duration
	timerTask     TaskItem
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

	// Time when the last tickMsg was handled, for debounce
	lastTickTime time.Time
}

// initialModel creates the initial TUI model.
func initialModel(tasks []TaskItem, markdownFile string, height int, stateMgr core.StateStore, states []state.TimeBoxState) model {
	items := make([]list.Item, len(tasks))
	for i, t := range tasks {
		ti := t
		// Set initial width to default 80 - padding
		ti.SetWidth(76)
		items[i] = ti
	}
	// Use a default height and width if not set yet
	listHeight := max(height-12, 5)
	defaultWidth := 80
	// Use multilineDelegate instead of default delegate
	listDelegate := &multilineDelegate{
		titleStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FFFF")),
		descStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
	}
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
		activeView:  ViewTaskList,
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
