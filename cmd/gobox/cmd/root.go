package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"gobox/internal/core" // Import our new internal/core package
	"gobox/internal/parser"

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
			tasks = append(tasks, TaskItem{
				title: t.Description,
				desc:  t.TimeBox,
			})
		}
		p := tea.NewProgram(initialModel(tasks))
		if err := p.Start(); err != nil {
			fmt.Println("Error running TUI:", err)
			os.Exit(1)
		}
	},
}

// TaskItem represents a task for the list.
type TaskItem struct {
	title, desc string
}

func (t TaskItem) Title() string       { return t.title }
func (t TaskItem) Description() string { return t.desc }
func (t TaskItem) FilterValue() string { return t.title }

// model is the Bubbletea model for the TUI.
type model struct {
	list     list.Model
	quitting bool
}

func initialModel(tasks []TaskItem) model {
	items := make([]list.Item, len(tasks))
	for i, t := range tasks {
		items[i] = t
	}
	l := list.New(items, list.NewDefaultDelegate(), 40, 10)
	l.Title = "GoBox Tasks"
	return model{list: l}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			// TODO: Start timebox for selected task
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
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
