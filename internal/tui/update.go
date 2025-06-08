package tui

import (
	"fmt"
	"strings"
	"time"

	"gobox/internal/gitwatcher"
	"gobox/internal/gitutil"
	"gobox/internal/parser"
	"gobox/internal/rewrite"
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
			// Only attempt to stop the session runner if we're in an active timer
			if m.timerActive {
				if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
					runner.Stop()
				}
			}
			_ = m.stateMgr.Save(m.states)
			return m, tea.Quit
		}
	
		if k == "enter" && m.timerActive {
			// Complete timer early when enter is pressed during active timer
			if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
				runner.Complete()
				m.timerActive = false
				m.timerDone = true
				return m, nil
			}
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
			if !m.timerDone {
				// For duration-based timers, count down
				if m.timerTotal > 0 {
					elapsed := runner.TotalElapsed()
					m.timer = m.timerTotal - elapsed
					if m.timer < 0 {
						m.timer = 0
					}
				} else {
					// For end-time based timers
					m.timer = runner.Remaining()
					if m.timer < 0 {
						m.timer = 0
					}
				}
			}
		}
		return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return tickMsg{}
		})
		
	case sessionCompletedMsg:
		m.timerActive = false
		m.timerDone = true
	
		// Save state when timer is completed
		if m.sessionState != nil {
			now := time.Now()
		
			// Ensure the last segment is closed
			if len(m.sessionState.Segments) > 0 && m.sessionState.Segments[len(m.sessionState.Segments)-1].End == nil {
				m.sessionState.Segments[len(m.sessionState.Segments)-1].End = &now
			}
		
			// Calculate the total duration spent on this task
			var totalDuration time.Duration
			for _, seg := range m.sessionState.Segments {
				if seg.End != nil {
					totalDuration += seg.End.Sub(seg.Start)
				} else {
					totalDuration += now.Sub(seg.Start)
				}
			}
		
			// Get any commits made during the task
			startTime := m.sessionState.Segments[0].Start
			_, err := gitutil.GetCommitsSince(startTime)
			if err != nil {
				// Handle git error silently - not all users have git repositories
			}
		
			// Don't update markdown file here - wait for user confirmation
			_ = m.stateMgr.Save(m.states)
		}
		return m, nil
		
	case commitMsg:
		// Add the commit message to our list
		m.commits = append(m.commits, string(msg))
		// Update the table rows
		rows := make([]table.Row, len(m.commits))
		for i, c := range m.commits {
			rows[i] = table.Row{c}
		}
		// Only update the table if we have at least one column
		if len(m.commitTable.Columns()) > 0 {
			m.commitTable.SetRows(rows)
		}
		return m, nil
		
	case tea.KeyMsg:
		if m.timerActive {
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
					// Safe stop that won't panic if channels are closed
					if m.timerActive {
						runner.Stop()
					}
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
			// Task is already completed in sessionCompletedMsg handler
			// Here we just handle returning to the task list
			
			// Let the user see the completion message before proceeding
			switch msg.String() {
			case "enter", " ":
				// User confirmed completion - now update the markdown file
				if m.sessionState != nil && m.list.Title != "" {
					now := time.Now()
					
					// Calculate total duration
					var totalDuration time.Duration
					for _, seg := range m.sessionState.Segments {
						if seg.End != nil {
							totalDuration += seg.End.Sub(seg.Start)
						} else {
							totalDuration += now.Sub(seg.Start)
						}
					}
					
					// Get commits for the task duration
					startTime := m.sessionState.Segments[0].Start
					commitsDuringTask, _ := gitutil.GetCommitsSince(startTime)
					
					// Update the markdown file
					updatedTask := m.timerTask.task
					updatedTask.IsChecked = true
				
					markdownFile := m.list.Title
				
					// Use the direct task update method that works with description matching
					updateErr := rewrite.UpdateTaskWithState(markdownFile, updatedTask, totalDuration, commitsDuringTask)
					if updateErr != nil {
						// Handle error silently - logging removed
					}
					
					// When returning to list, remove the task from states
					m.states = m.stateMgr.RemoveTaskState(m.states, m.sessionState.TaskHash)
					_ = m.stateMgr.Save(m.states)
					m.sessionState = nil
				}
				
				m.timerDone = false
			default:
				return m, nil
			}
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
						m.timerTask = item
						
						// Start the runner
						runner.Start()
						
						// Initialize git watcher if not already set up
						if m.gitWatcher == nil {
							// Use the same time format as the core package does
							watcher := gitwatcher.NewGitWatcher(now, 5*time.Second)
							m.gitWatcher = watcher
							watcher.Start()
							
							// Make sure the commit table is initialized with proper columns
							if len(m.commitTable.Columns()) == 0 {
								columns := []table.Column{
									{Title: "Commit", Width: m.width - 4},
								}
								m.commitTable = table.New(
									table.WithColumns(columns),
									table.WithRows([]table.Row{}),
									table.WithFocused(false),
									table.WithHeight(10),
								)
							}
						}
						
						// Return commands for handling session tick events and git commits
						var cmds []tea.Cmd
						cmds = append(cmds, sessionTickCmd(runner))
						if watcher, ok := m.gitWatcher.(*gitwatcher.GitWatcher); ok && watcher != nil {
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