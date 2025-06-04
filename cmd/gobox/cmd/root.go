package cmd

import (
	"os"
	"time"

	"github.com/spf13/cobra"

	"gobox/internal/core" // Import our new internal/core package
	"gobox/internal/parser"
	"gobox/pkg/task"

	"fmt"

	"github.com/charmbracelet/bubbles/list"
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
	list        list.Model
	quitting    bool
	timerActive bool
	timer       time.Duration
	timerTotal  time.Duration
	timerTask   TaskItem
	timerDone   bool
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

func tick() tea.Msg {
	time.Sleep(1 * time.Second)
	return tickMsg{}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.timerActive {
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			case "enter":
				// Complete timer early
				m.timerActive = false
				m.timerDone = true
				return m, nil
			}
		} else if m.timerDone {
			// Mark the task as checked in the Markdown file
			updated := m.timerTask.task
			updated.IsChecked = true
			err := parser.UpdateMarkdown(m.list.Title, updated, nil, m.timerTotal)
			if err != nil {
				fmt.Println("Error updating markdown:", err)
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
				// Start timer for selected task
				if item, ok := m.list.SelectedItem().(TaskItem); ok {
					duration, endTime, err := parser.ParseTimeBox(item.task.TimeBox)
					if err == nil && (duration > 0 || !endTime.IsZero()) {
						var timerDuration time.Duration
						if duration > 0 {
							timerDuration = duration
						} else {
							timerDuration = time.Until(endTime)
						}
						m.timerActive = true
						m.timer = timerDuration
						m.timerTotal = timerDuration
						m.timerTask = item
						m.timerDone = false
						return m, tick
					}
				}
				return m, nil
			}
		}
	case tickMsg:
		if m.timerActive {
			if m.timer > time.Second {
				m.timer -= time.Second
				return m, tick
			} else {
				m.timer = 0
				m.timerActive = false
				m.timerDone = true
				return m, nil
			}
		}
	}
	if !m.timerActive {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	return m, nil
}

// (removed parseDurationFromLine; now using parser.ParseTimeBox)

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}
	if m.timerActive {
		return lipgloss.NewStyle().Padding(1).Render(
			fmt.Sprintf(
				"Working on: %s\nTime remaining: %s\n\nPress Enter to complete early.",
				m.timerTask.line,
				m.timer.Round(time.Second).String(),
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
