package tui

import (
	"fmt"
	"strings"
	"time"

	"gobox/internal/gitwatcher"
	"gobox/internal/parser"
	"gobox/internal/session"
	"gobox/internal/state"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
)

// Message types for Bubbletea update loop
type tickMsg struct{}
type sessionCompletedMsg struct{}
type commitMsg string

// sessionTickCmd returns a Bubbletea command that listens for session runner events.
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

// watchCommitsCmd returns a Bubbletea command that listens for new git commits or errors.
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

// Update handles all Bubbletea update logic for the TUI model.
func Update(m model, msg tea.Msg) (model, tea.Cmd) {
	// Always handle quit first to ensure TUI can exit
	if msg, ok := msg.(tea.KeyMsg); ok {
		k := msg.String()
		if k == "ctrl+c" || k == "q" {
			m.quitting = true
			if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
				runner.Stop()
			}
			_ = m.stateMgr.Save(m.states)
			return m, tea.Quit
		}
	}
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
		
	case tickMsg:
		// Update timer display
		if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
			m.timer = runner.Remaining()
			if m.timer < 0 {
				m.timer = 0
			}
		}
		return m, nil
		
	case sessionCompletedMsg:
		m.timerActive = false
		m.timerDone = true
		return m, nil
		
	case commitMsg:
		// Add the commit message to our list
		m.commits = append(m.commits, string(msg))
		// Update the table rows
		rows := make([]table.Row, len(m.commits))
		for i, c := range m.commits {
			rows[i] = table.Row{c}
		}
		m.commitTable.SetRows(rows)
		return m, nil
		
	case tea.KeyMsg:
		if m.timerActive {
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
					runner.Stop()
				}
				_ = m.stateMgr.Save(m.states)
				return m, tea.Quit
			case "enter":
				// Complete timer early
				if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
					runner.Complete()
				}
				return m, nil
			}
		} else if m.timerDone {
			// Mark the task as checked in the Markdown file using core.CompleteTask with TimeBoxState
			if m.sessionState != nil {
				// This should be handled by a callback or in the main tui.go Run function.
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
				now := time.Now()
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
						now := time.Now()
						taskHash := item.task.Hash()
						found := false
						var idx int
						
						// Find existing task state or create a new one
						for i := range m.states {
							if m.states[i].TaskHash == taskHash {
								idx = i
								found = true
								break
							}
						}
						
						if !found {
							// Get a clean state by removing any existing task state with the same hash
							cleanStates := m.stateMgr.RemoveTaskState(m.states, taskHash)
							
							// Create a new state for this task
							newState := state.TimeBoxState{
								TaskHash: taskHash,
								Segments: []state.TimeSegment{{Start: now}},
							}
							
							// Add the new state
							m.states = append(cleanStates, newState)
							idx = len(m.states) - 1
							
							// Save state to disk
							_ = m.stateMgr.Save(m.states)
						} else {
							// Add a new segment to the existing task state
							segment := state.TimeSegment{Start: now}
							m.states[idx].Segments = append(m.states[idx].Segments, segment)
						}
						
						// Set current session state
						m.sessionState = &m.states[idx]
						
						// Set up timer
						m.timerTask = item
						m.timerActive = true
						m.timerDone = false
						
						// Initialize session runner
						runner := session.NewSessionRunner(item.task, m.sessionState, duration, endTime)
						m.sessionRunner = runner
						m.timerTotal = duration
						m.timer = duration
						
						// Start the runner
						runner.Start()
						
						// Initialize git watcher if not already set up
						if m.gitWatcher == nil {
							watcher := gitwatcher.NewGitWatcher(now, 5*time.Second)
							m.gitWatcher = watcher
							watcher.Start()
						}
						
						// Return commands for handling session tick events and git commits
						var cmds []tea.Cmd
						cmds = append(cmds, sessionTickCmd(runner))
						if watcher, ok := m.gitWatcher.(*gitwatcher.GitWatcher); ok {
							cmds = append(cmds, watchCommitsCmd(watcher))
						}
						
						return m, tea.Batch(cmds...)
					}
				}
			}
		}
	}
	
	// Forward key events to the list when not in timer mode
	if !m.timerActive && !m.timerDone {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	
	return m, nil
}