package tui

import (
	"fmt"
	"time"

	"gobox/internal/gitutil"
	"gobox/internal/gitwatcher"
	"gobox/internal/parser"
	"gobox/internal/rewrite"
	"gobox/internal/session"
	"gobox/internal/state"

	"slices"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
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
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleKeyMsg(m, msg)
	case tickMsg:
		return handleTickMsg(m, msg)
	case sessionCompletedMsg:
		return handleSessionCompletedMsg(m, msg)
	case commitMsg:
		return handleCommitMsg(m, msg)
	case tea.WindowSizeMsg:
		return handleWindowResize(m, msg)
	default:
		if m.activeView == ViewTaskList {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func handleKeyMsg(m model, msg tea.KeyMsg) (model, tea.Cmd) {
	k := msg.String()

	switch m.activeView {
	case ViewQuitting:
		// If quitting, ignore further input
		return m, nil

	case ViewTimerActive:
		switch k {
		case "ctrl+c", "q":
			m.activeView = ViewQuitting
			if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
				runner.Stop()
			}
			_ = m.stateMgr.Save(m.states)
			return m, tea.Quit

		case "enter":
			if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
				runner.Complete()
			}
			m.activeView = ViewTimerDone
			return m, nil
		}

	case ViewTimerDone:
		switch k {
		case "enter", " ":
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
				commitsDuringTask, _ := func() ([]string, error) {
					var allCommits []string
					commitSet := make(map[string]struct{})

					for _, seg := range m.sessionState.Segments {
						if seg.End == nil {
							continue
						}
						commits, err := gitutil.GetCommitsBetweenTimeRange(seg.Start, *seg.End)
						if err != nil {
							return nil, err
						}
						for _, c := range commits {
							if _, exists := commitSet[c]; !exists {
								commitSet[c] = struct{}{}
								allCommits = append(allCommits, c)
							}
						}
					}
					return allCommits, nil
				}()

				// Update the markdown file
				updatedTask := m.timerTask.task
				updatedTask.IsChecked = true

				markdownFile := m.list.Title

				// Use the direct task update method that works with description matching
				err := rewrite.UpdateTaskWithState(markdownFile, updatedTask, totalDuration, commitsDuringTask)
				if err != nil {
					// silently ignore
				}

				// Remove completed task state and save
				m.states = m.stateMgr.RemoveTaskState(m.states, m.sessionState.TaskHash)
				_ = m.stateMgr.Save(m.states)
				m.sessionState = nil
			}

			m.activeView = ViewTaskList
			return m, nil

		default:
			return m, nil
		}

	case ViewTaskList:
		switch k {
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
			m.activeView = ViewQuitting
			return m, tea.Quit

		case "enter":
			if item, ok := m.list.SelectedItem().(TaskItem); ok {
				duration, endTime, err := parser.ParseTimeBox(item.task.TimeBox)
				if err == nil && (duration > 0 || !endTime.IsZero()) {
					now := time.Now()
					taskHash := item.task.Hash()
					found := false
					var idx int

					// Find existing task state or create new one
					for i := range m.states {
						if m.states[i].TaskHash == taskHash {
							idx = i
							found = true
							break
						}
					}
					if !found {
						cleanStates := m.stateMgr.RemoveTaskState(m.states, taskHash)
						newState := state.TimeBoxState{
							TaskHash: taskHash,
							Segments: []state.TimeSegment{{Start: now}},
						}
						m.states = append(cleanStates, newState)
						idx = len(m.states) - 1
						_ = m.stateMgr.Save(m.states)
					} else {
						segment := state.TimeSegment{Start: now}
						m.states[idx].Segments = append(m.states[idx].Segments, segment)
					}

					m.sessionState = &m.states[idx]

					// Set up timer state
					m.timerTask = item
					m.activeView = ViewTimerActive

					runner := session.NewSessionRunner(item.task, m.sessionState, duration, endTime)
					m.sessionRunner = runner
					m.timerTotal = duration
					m.timer = duration
					m.timerTask = item

					runner.Start()

					// Setup git watcher if needed
					if m.gitWatcher == nil {
						var startTime time.Time
						if len(m.sessionState.Segments) > 0 {
							startTime = m.sessionState.Segments[0].Start
						} else {
							startTime = now
						}
						watcher := gitwatcher.NewGitWatcher(startTime, 5*time.Second)
						m.gitWatcher = watcher

						if len(m.sessionState.Segments) > 1 {
							commitSet := make(map[string]struct{})
							for _, seg := range m.sessionState.Segments {
								if seg.End == nil {
									continue
								}
								commits, err := gitutil.GetCommitsBetweenTimeRange(seg.Start, *seg.End)
								if err != nil {
									continue
								}
								for _, commit := range commits {
									if _, exists := commitSet[commit]; !exists {
										commitSet[commit] = struct{}{}
										m.commits = append(m.commits, commit)
									}
								}
							}
							if len(m.commits) > 0 {
								rows := make([]table.Row, len(m.commits))
								for i, c := range m.commits {
									rows[i] = table.Row{c}
								}
								if len(m.commitTable.Columns()) > 0 {
									m.commitTable.SetRows(rows)
								}
							}
						}

						watcher.Start()

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

					cmds := []tea.Cmd{sessionTickCmd(runner)}
					if watcher, ok := m.gitWatcher.(*gitwatcher.GitWatcher); ok && watcher != nil {
						cmds = append(cmds, watchCommitsCmd(watcher))
					}
					return m, tea.Batch(cmds...)
				}
			}
		default:
			// Forward other keys to the list's update for navigation, selection, etc.
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func handleTickMsg(m model, _ tickMsg) (model, tea.Cmd) {
	if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
		if m.activeView != ViewTimerDone {
			if m.timerTotal > 0 {
				elapsed := runner.TotalElapsed()
				m.timer = m.timerTotal - elapsed
				if m.timer < 0 {
					m.timer = 0
				}
			} else {
				m.timer = runner.Remaining()
				if m.timer < 0 {
					m.timer = 0
				}
			}
		}
	}

	return m, tea.Tick(time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func handleSessionCompletedMsg(m model, _ sessionCompletedMsg) (model, tea.Cmd) {
	m.activeView = ViewTimerDone

	if m.sessionState != nil {
		now := time.Now()

		// Close last segment if open
		if len(m.sessionState.Segments) > 0 && m.sessionState.Segments[len(m.sessionState.Segments)-1].End == nil {
			m.sessionState.Segments[len(m.sessionState.Segments)-1].End = &now
		}

		// Calculate total duration
		var totalDuration time.Duration
		for _, seg := range m.sessionState.Segments {
			if seg.End != nil {
				totalDuration += seg.End.Sub(seg.Start)
			} else {
				totalDuration += now.Sub(seg.Start)
			}
		}

		// Attempt to get commits, ignore errors
		_, _ = func() ([]string, error) {
			var allCommits []string
			commitSet := make(map[string]struct{})

			for _, seg := range m.sessionState.Segments {
				if seg.End == nil {
					continue
				}
				commits, err := gitutil.GetCommitsBetweenTimeRange(seg.Start, *seg.End)
				if err != nil {
					return nil, err
				}
				for _, c := range commits {
					if _, exists := commitSet[c]; !exists {
						commitSet[c] = struct{}{}
						allCommits = append(allCommits, c)
					}
				}
			}
			return allCommits, nil
		}()

		_ = m.stateMgr.Save(m.states)
	}
	return m, nil
}

func handleCommitMsg(m model, msg commitMsg) (model, tea.Cmd) {
	newCommit := string(msg)
	isDuplicate := slices.Contains(m.commits, newCommit)

	if !isDuplicate {
		m.commits = append(m.commits, newCommit)
		rows := make([]table.Row, len(m.commits))
		for i, c := range m.commits {
			rows[i] = table.Row{c}
		}
		if len(m.commitTable.Columns()) > 0 {
			m.commitTable.SetRows(rows)
		}
	}

	if watcher, ok := m.gitWatcher.(*gitwatcher.GitWatcher); ok && watcher != nil {
		return m, watchCommitsCmd(watcher)
	}
	return m, nil
}

func handleWindowResize(m model, msg tea.WindowSizeMsg) (model, tea.Cmd) {
	m.height = msg.Height
	m.width = msg.Width
	listHeight := max(msg.Height-12, 5)
	m.list.SetHeight(listHeight)
	m.list.SetWidth(msg.Width)
	m.commitTable.SetHeight(10)
	m.commitTable.SetWidth(msg.Width)

	// Refresh list items with updated width for dynamic wrapping
	items := make([]list.Item, len(m.list.Items()))
	for i := 0; i < len(m.list.Items()); i++ {
		if taskItem, ok := m.list.Items()[i].(TaskItem); ok {
			ti := taskItem
			ti.SetWidth(m.width - 4) // Subtract any padding
			items[i] = ti
		} else {
			items[i] = m.list.Items()[i]
		}
	}
	m.list.SetItems(items)

	return m, nil
}
