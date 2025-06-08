package tui

import (
	"fmt"
	"strings"
	"time"

	"gobox/internal/core"
	"gobox/internal/gitwatcher"
	"gobox/internal/parser"
	"gobox/internal/session"
	"gobox/internal/state"
	"gobox/pkg/task"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	sessionRunner *session.SessionRunner
	sessionState  *state.TimeBoxState
	gitWatcher    *gitwatcher.GitWatcher
	commits       []string
	commitTable   table.Model
	height        int // Track terminal height for dynamic resizing
	width         int // Track terminal width for dynamic resizing

	// State file support
	stateMgr core.StateStore
	states   []state.TimeBoxState
}

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
	m := model{
		list:     l,
		height:   height,
		width:    defaultWidth,
		stateMgr: stateMgr,
		states:   states,
	}
	return m
}

func (m model) Init() tea.Cmd {
	return tea.EnterAltScreen
}

type tickMsg struct{}
type sessionCompletedMsg struct{}
type commitMsg string

func sessionTickCmd(runner *session.SessionRunner) tea.Cmd {
	return func() tea.Msg {
		for {
			ev := <-runner.Events()
			switch ev {
			case session.EventTick:
				return tickMsg{}
			case session.EventCompleted:
				return sessionCompletedMsg{}
			}
		}
	}
}

func watchCommitsCmd(gw *gitwatcher.GitWatcher) tea.Cmd {
	return func() tea.Msg {
		select {
		case commit := <-gw.Commits():
			return commitMsg(commit)
		case err := <-gw.Errors():
			return commitMsg(fmt.Sprintf("Git error: %v", err))
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		listHeight := max(msg.Height-12, 5)
		m.list.SetHeight(listHeight)
		m.list.SetWidth(msg.Width)
		m.commitTable.SetHeight(10)
		m.commitTable.SetWidth(msg.Width)
		return m, nil
	case tea.KeyMsg:
		if m.timerActive {
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				if m.sessionRunner != nil {
					m.sessionRunner.Pause()
				}
				m.stateMgr.Save(m.states)
				return m, tea.Quit
			case "enter":
				// Complete timer early
				if m.sessionRunner != nil {
					m.sessionRunner.Complete()
				}
				return m, nil
			}
		} else if m.timerDone {
			// Mark the task as checked in the Markdown file using core.CompleteTask with TimeBoxState
			if m.sessionState != nil {
				err := core.CompleteTask(m.list.Title, m.timerTask.task, *m.sessionState, nil)
				if err != nil {
					fmt.Println("Error updating markdown:", err)
				}
				// Remove state for this task and save
				taskHash := m.timerTask.task.Hash()
				m.states = m.stateMgr.RemoveTaskState(m.states, taskHash)
				_ = m.stateMgr.Save(m.states)
			}
			m.timerDone = false
			// Reload tasks from markdown file
			if m.list.Title != "" {
				parsedTasks, err := parser.ParseMarkdownFile(m.list.Title)
				if err == nil {
					tasks := make([]TaskItem, 0, len(parsedTasks))
					for _, t := range parsedTasks {
						if t.IsChecked {
							continue // Skip checked tasks in the TUI
						}
						line := fmt.Sprintf("%s %s", strings.TrimSpace(t.Description), strings.TrimSpace(t.TimeBox))
						tasks = append(tasks, TaskItem{line: line, task: t})
					}
					items := make([]list.Item, len(tasks))
					for i, t := range tasks {
						items[i] = t
					}
					m.list.SetItems(items)
				}
			}
			return m, nil
		} else {
			switch msg.String() {
			case "ctrl+c", "q":
				// Pause (close) current time segment and save state on quit, matching CLI
				now := time.Now()
				// Re-find the sessionState pointer in case m.states was reassigned
				if m.sessionState != nil {
					taskHash := m.sessionState.TaskHash
					for i := range m.states {
						if m.states[i].TaskHash == taskHash {
							m.sessionState = &m.states[i]
							break
						}
					}
				}
				if m.sessionState != nil && len(m.sessionState.Segments) > 0 {
					lastSeg := &m.sessionState.Segments[len(m.sessionState.Segments)-1]
					if lastSeg.End == nil {
						lastSeg.End = &now
					}
				}
				_ = m.stateMgr.Save(m.states)
				m.quitting = true
				return m, tea.Quit
			case "enter":
				// Start timer for selected task using SessionRunner
				if item, ok := m.list.SelectedItem().(TaskItem); ok {
					duration, endTime, err := parser.ParseTimeBox(item.task.TimeBox)
					if err == nil && (duration > 0 || !endTime.IsZero()) {
						// --- State file: find or create state for this task
						now := time.Now()
						taskHash := item.task.Hash()
						found := false
						var idx int
						for i := range m.states {
							if m.states[i].TaskHash == taskHash {
								idx = i
								found = true
								break
							}
						}
						if !found {
							m.states = append(m.states, state.TimeBoxState{
								TaskHash: taskHash,
								Segments: []state.TimeSegment{{Start: now, End: nil}},
							})
							idx = len(m.states) - 1
						} else if len(m.states[idx].Segments) == 0 || m.states[idx].Segments[len(m.states[idx].Segments)-1].End != nil {
							m.states[idx].Segments = append(m.states[idx].Segments, state.TimeSegment{Start: now, End: nil})
						}
						m.sessionState = &m.states[idx]
						_ = m.stateMgr.Save(m.states)
						// --- End state file logic

						runner := session.NewSessionRunner(item.task, m.sessionState, duration, endTime)
						m.sessionRunner = runner
						m.timerActive = true
						m.timerDone = false
						m.timerTask = item
						runner.Start()
						// Start GitWatcher
						gw := gitwatcher.NewGitWatcher(time.Now(), 5*time.Second)
						m.gitWatcher = gw
						m.commits = nil
						// Initialize commitTable
						columns := []table.Column{
							{Title: "Hash", Width: 10},
							{Title: "Message", Width: 60},
						}
						m.commitTable = table.New(
							table.WithColumns(columns),
							table.WithRows([]table.Row{}),
							table.WithFocused(false),
						)
						m.commitTable.SetWidth(m.width)
						m.commitTable.SetHeight(10)
						gw.Start()
						return m, tea.Batch(sessionTickCmd(runner), watchCommitsCmd(gw), tea.ClearScreen)
					}
				}
				return m, nil
			}
		}
	case tickMsg:
		if m.timerActive && m.sessionRunner != nil {
			// Update timer from sessionRunner
			m.timer = m.sessionRunner.Duration
			elapsed := m.sessionRunner.TotalElapsed()
			if m.timer > elapsed {
				m.timer = m.timer - elapsed
			} else {
				m.timer = 0
			}
			return m, tea.Batch(sessionTickCmd(m.sessionRunner), watchCommitsCmd(m.gitWatcher))
		}
	case commitMsg:
		// Add new commit to the list and table
		commit := string(msg)
		m.commits = append(m.commits, commit)
		parts := strings.SplitN(commit, " ", 2)
		hash := parts[0]
		msgStr := ""
		if len(parts) > 1 {
			msgStr = parts[1]
		}
		rows := append(m.commitTable.Rows(), table.Row{hash, msgStr})
		m.commitTable.SetRows(rows)
		// Only blur/focus on explicit user action (not here)
		if len(rows) > 0 {
			m.commitTable.SetCursor(len(rows) - 1)
		}
		return m, watchCommitsCmd(m.gitWatcher)
	case sessionCompletedMsg:
		m.timerActive = false
		m.timerDone = true
		// Stop GitWatcher
		if m.gitWatcher != nil {
			m.gitWatcher.Stop()
		}
		if m.quitting {
			// Pause (close) current time segment and save state on quit, matching CLI
			now := time.Now()
			// Re-find the sessionState pointer in case m.states was reassigned
			if m.sessionState != nil {
				taskHash := m.sessionState.TaskHash
				for i := range m.states {
					if m.states[i].TaskHash == taskHash {
						m.sessionState = &m.states[i]
						break
					}
				}
			}
			if m.sessionState != nil && len(m.sessionState.Segments) > 0 {
				lastSeg := &m.sessionState.Segments[len(m.sessionState.Segments)-1]
				if lastSeg.End == nil {
					lastSeg.End = &now
				}
			}
			_ = m.stateMgr.Save(m.states)
			return m, tea.Quit
		}
		return m, nil
	}
	if !m.timerActive {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		// Always update the commitTable as well, so it can render/focus/scroll if needed
		var tableCmd tea.Cmd
		m.commitTable, tableCmd = m.commitTable.Update(msg)
		return m, tea.Batch(cmd, tableCmd)
	}
	// Even if timer is active, update the commitTable for every message
	var tableCmd tea.Cmd
	m.commitTable, tableCmd = m.commitTable.Update(msg)
	return m, tableCmd
}

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}
	if m.timerActive {
		timerBlock := lipgloss.NewStyle().Padding(1).Render(
			fmt.Sprintf(
				"Working on: %s\nTime remaining: %s\n\nPress Enter to complete early.",
				m.timerTask.line,
				m.timer.Round(time.Second).String(),
			),
		)
		commitsBlock := lipgloss.NewStyle().Padding(1).Render("Commits during session:")
		commitTableBlock := m.commitTable.View()

		content := lipgloss.JoinVertical(lipgloss.Left, timerBlock, commitsBlock, commitTableBlock)
		contentLines := strings.Count(content, "\n") + 1
		if m.height > contentLines {
			content += strings.Repeat("\n", m.height-contentLines)
		}
		return content
	}
	if m.timerDone {
		// Show completion message and return to list after a keypress
		m2 := m
		m2.timerDone = false
		return lipgloss.NewStyle().Padding(1).Render(
			fmt.Sprintf("Task completed!\n\nPress any key to return to the list."),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Padding(1).Render(m.list.View()),
		m.commitTable.View(),
	)
}

// max returns the maximum of two ints.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Run launches the GoBox TUI for the given markdown file, state manager, and state.
func Run(markdownFile string, stateMgr core.StateStore, states []state.TimeBoxState) error {
	parsedTasks, err := parser.ParseMarkdownFile(markdownFile)
	if err != nil {
		return fmt.Errorf("Error loading tasks from markdown: %w", err)
	}

	tasks := make([]TaskItem, 0, len(parsedTasks))
	for _, t := range parsedTasks {
		if t.IsChecked {
			continue // Skip checked tasks in the TUI
		}
		line := fmt.Sprintf("%s %s", strings.TrimSpace(t.Description), strings.TrimSpace(t.TimeBox))
		tasks = append(tasks, TaskItem{line: line, task: t})
	}
	// Use a default height (will be updated on first WindowSizeMsg)
	p := tea.NewProgram(initialModel(tasks, markdownFile, 24, stateMgr, states))

	_, err = p.Run()
	return err
}
