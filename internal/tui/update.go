package tui

import (
	"fmt"
	"time"

	"gobox/internal/gitutil"
	"gobox/internal/gitwatcher"
	"gobox/internal/parser"
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
type reloadListMsg struct{}

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
		return HandleKeyMsg(m, msg)
	case reloadListMsg:
		return handleReloadListMsg(m, msg)
	case tickMsg:
		return handleTickMsg(m, msg)
	case sessionCompletedMsg:
		return handleSessionCompletedMsg(m, msg)
	case commitMsg:
		return handleCommitMsg(m, msg)
	case tea.WindowSizeMsg:
		return handleWindowResize(m, msg)
	default:
		if m.ActiveView == ViewTaskList {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func HandleKeyMsg(m model, msg tea.KeyMsg) (model, tea.Cmd) {
	k := msg.String()

	switch m.ActiveView {
	case ViewQuitting:
		// If quitting, ignore further input
		return m, nil

	case ViewTimerActive:
		switch k {
		case "ctrl+c", "q":
			m.ActiveView = ViewQuitting
			if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
				runner.Stop()
			}
			_ = m.stateMgr.Save(m.States)
			return m, tea.Quit

		case "enter":
			if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
				runner.Complete()
			}
			m.ActiveView = ViewTimerDone
			return m, nil
		}

	case ViewTimerDone:
		switch k {
		case "enter", " ":
			if m.SessionState != nil && m.list.Title != "" {
				now := time.Now()

				// Calculate total duration
				var totalDuration time.Duration
				for _, seg := range m.SessionState.Segments {
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

					for _, seg := range m.SessionState.Segments {
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
				updatedTask := m.TimerTask.Task
				updatedTask.IsChecked = true

				markdownFile := m.list.Title

				if err := parser.UpdateMarkdown(markdownFile, updatedTask, commitsDuringTask, totalDuration); err != nil {
					fmt.Printf("Failed to update markdown file %s, quitting\n", markdownFile)
					return m, tea.Quit
				}

				// Remove completed task state and save
				m.States = m.stateMgr.RemoveTaskState(m.States, m.SessionState.TaskHash)
				_ = m.stateMgr.Save(m.States)
				m.SessionState = nil
			}

			m.ActiveView = ViewTaskList
			return m, func() tea.Msg { return reloadListMsg{} }

		default:
			return m, nil
		}

	case ViewTaskList:
		switch k {
		case "ctrl+c", "q":
			now := time.Now()
			if m.SessionState != nil {
				taskHash := m.SessionState.TaskHash
				for i := range m.States {
					if m.States[i].TaskHash == taskHash {
						m.SessionState = &m.States[i]
						break
					}
				}
			}
			if m.SessionState != nil && len(m.SessionState.Segments) > 0 {
				lastSeg := &m.SessionState.Segments[len(m.SessionState.Segments)-1]
				if lastSeg.End == nil {
					lastSeg.End = &now
				}
			}
			_ = m.stateMgr.Save(m.States)
			m.ActiveView = ViewQuitting
			return m, tea.Quit

		case "enter":
			if item, ok := m.list.SelectedItem().(TaskItem); ok {
				duration, endTime, err := parser.ParseTimeBox(item.Task.TimeBox)
				if err == nil && (duration > 0 || !endTime.IsZero()) {
					now := time.Now()
					taskHash := item.Task.Hash()
					found := false
					var idx int

					// Find existing task state or create new one
					for i := range m.States {
						if m.States[i].TaskHash == taskHash {
							idx = i
							found = true
							break
						}
					}
					if !found {
						cleanStates := m.stateMgr.RemoveTaskState(m.States, taskHash)
						newState := state.TimeBoxState{
							TaskHash: taskHash,
							Segments: []state.TimeSegment{{Start: now}},
						}
						m.States = append(cleanStates, newState)
						idx = len(m.States) - 1
						_ = m.stateMgr.Save(m.States)
					} else {
						segment := state.TimeSegment{Start: now}
						m.States[idx].Segments = append(m.States[idx].Segments, segment)
					}

					m.SessionState = &m.States[idx]

					// Set up timer state
					m.TimerTask = item
					m.ActiveView = ViewTimerActive

					runner := session.NewSessionRunner(item.Task, m.SessionState, duration, endTime)
					m.sessionRunner = runner
					m.timerTotal = duration
					m.timer = duration
					m.TimerTask = item

					runner.Start()

					// Setup git watcher if needed
					if m.gitWatcher == nil {
						var startTime time.Time
						if len(m.SessionState.Segments) > 0 {
							startTime = m.SessionState.Segments[0].Start
						} else {
							startTime = now
						}
						watcher := gitwatcher.NewGitWatcher(startTime, 5*time.Second)
						m.gitWatcher = watcher

						if len(m.SessionState.Segments) > 1 {
							commitSet := make(map[string]struct{})
							for _, seg := range m.SessionState.Segments {
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

func handleReloadListMsg(m model, _ reloadListMsg) (model, tea.Cmd) {
	tasks, err := parser.ParseMarkdownFile(m.list.Title)
	if err == nil {
		var taskItems []TaskItem
		for _, t := range tasks {
			if !t.IsChecked {
				taskItems = append(taskItems, TaskItem{
					RawLine: t.String(),
					Task:    t,
					Width:   m.width - 4,
				})
			}
		}
		m.list = initList(taskItems, m.list.Title, m.height)
	}
	return m, nil
}

func handleTickMsg(m model, _ tickMsg) (model, tea.Cmd) {
	if runner, ok := m.sessionRunner.(*session.SessionRunner); ok && runner != nil {
		if m.ActiveView != ViewTimerDone {
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
	m.ActiveView = ViewTimerDone

	if m.SessionState != nil {
		now := time.Now()

		// Close last segment if open
		if len(m.SessionState.Segments) > 0 && m.SessionState.Segments[len(m.SessionState.Segments)-1].End == nil {
			m.SessionState.Segments[len(m.SessionState.Segments)-1].End = &now
		}

		// Calculate total duration
		var totalDuration time.Duration
		for _, seg := range m.SessionState.Segments {
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

			for _, seg := range m.SessionState.Segments {
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

		_ = m.stateMgr.Save(m.States)
		tasks, err := parser.ParseMarkdownFile(m.list.Title)
		if err == nil {
			var items []list.Item
			for _, t := range tasks {
				if !t.IsChecked {
					items = append(items, TaskItem{
						RawLine: t.String(),
						Task:    t,
						Width:   m.width - 4,
					})
				}
			}
			m.list.SetItems(items)
		}
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
	for i := range m.list.Items() {
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
