package cmd

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gobox/internal/core" // Import our new internal/core package
	"gobox/internal/gitwatcher"
	"gobox/internal/parser"
	"gobox/internal/session"
	"gobox/internal/state"
	"gobox/pkg/task"

	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gobox [markdown_file]",
	Short: "A tiny CLI tool for timeboxing tasks in Markdown files with Git integration",
	Long: `gobox parses a markdown file, starts a timer for the next unchecked task with a timebox,
updates the markdown upon completion with a checkmark and Git commits.`,
	Args: cobra.ExactArgs(1), // Expect exactly one argument: the markdown file path
	Run: func(cmd *cobra.Command, args []string) {
		markdownFile := args[0] // Get the markdown file path from Cobra arguments

		// Call the main application logic from the internal/core package
		core.StartGoBox(markdownFile)
	},
}

// TUI subcommand
var tuiCmd = &cobra.Command{
	Use:   "tui [markdown_file]",
	Short: "Launch the GoBox interactive TUI",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		markdownFile := args[0]
		parsedTasks, err := parser.ParseMarkdownFile(markdownFile)

		if err != nil {
			fmt.Println("Error loading tasks from markdown:", err)
			os.Exit(1)
		}

		tasks := make([]TaskItem, 0, len(parsedTasks))
		for _, t := range parsedTasks {
			if t.IsChecked {
				continue // Skip checked tasks in the TUI
			}

			line := fmt.Sprintf("%s %s", t.Description, t.TimeBox)
			tasks = append(tasks, TaskItem{line: line, task: t})
		}
		p := tea.NewProgram(initialModel(tasks, markdownFile))

		if _, err := p.Run(); err != nil {
			fmt.Println("Error running TUI:", err)
			os.Exit(1)
		}
	},
}

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
}

func initialModel(tasks []TaskItem, markdownFile string) model {
	items := make([]list.Item, len(tasks))
	for i, t := range tasks {
		items[i] = t
	}
	l := list.New(items, list.NewDefaultDelegate(), 40, 10)
	l.Title = markdownFile // Store the file path in the title for reloads
	return model{list: l}
}

func (m model) Init() tea.Cmd {
	return nil
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
	case tea.KeyMsg:
		if m.timerActive {
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				if m.sessionRunner != nil {
					m.sessionRunner.Stop()
				}
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
						line := fmt.Sprintf("%s %s", t.Description, t.TimeBox)
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
				m.quitting = true
				return m, tea.Quit
			case "enter":
				// Start timer for selected task using SessionRunner
				if item, ok := m.list.SelectedItem().(TaskItem); ok {
					duration, endTime, err := parser.ParseTimeBox(item.task.TimeBox)
					if err == nil && (duration > 0 || !endTime.IsZero()) {
						tbState := &state.TimeBoxState{TaskHash: item.task.Hash()}
						runner := session.NewSessionRunner(item.task, tbState, duration, endTime)
						m.sessionRunner = runner
						m.sessionState = tbState
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
						gw.Start()
						return m, tea.Batch(sessionTickCmd(runner), watchCommitsCmd(gw))
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
		m.commitTable.Blur() // Ensure table is unfocused so it doesn't take over the UI
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
		return m, nil
	}
	if !m.timerActive {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	return m, nil
}

// (removed parseDurationFromLine; now using parser.ParseTimeBox)

// formatCommits is no longer needed; using table.Model for commit display.

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}
	if m.timerActive {
		commitSection := ""
		if m.commitTable.Rows() != nil && len(m.commitTable.Rows()) > 0 {
			commitSection = "\nCommits during session:\n" + m.commitTable.View()
		}
		return lipgloss.NewStyle().Padding(1).Render(
			fmt.Sprintf(
				"Working on: %s\nTime remaining: %s\n\nPress Enter to complete early.%s",
				m.timerTask.line,
				m.timer.Round(time.Second).String(),
				commitSection,
			),
		)
	}
	if m.timerDone {
		// Show completion message and return to list after a keypress
		m2 := m
		m2.timerDone = false
		return lipgloss.NewStyle().Padding(1).Render(
			fmt.Sprintf("Task completed!\n\nPress any key to return to the list."),
		)
	}
	return lipgloss.NewStyle().Padding(1).Render(m.list.View())
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Any global flags or initializations can go here.
	rootCmd.AddCommand(tuiCmd)
}
