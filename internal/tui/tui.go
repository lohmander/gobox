package tui

import (
	"fmt"
	"gobox/internal/core"
	"gobox/internal/parser"
	"gobox/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

// Init initializes the TUI model and returns any initial commands to run.
func (m model) Init() tea.Cmd {
	return nil
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
			continue
		}
		line := fmt.Sprintf("%s %s", t.Description, t.TimeBox)
		tasks = append(tasks, TaskItem{line: line, task: t})
	}

	m := initialModel(tasks, markdownFile, 24, stateMgr, states)
	p := tea.NewProgram(&teaModelAdapter{m})

	_, err = p.Run()
	return err
}

// teaModelAdapter adapts our model to the tea.Model interface using Update and ModelView.
type teaModelAdapter struct {
	m model
}

func (a *teaModelAdapter) Init() tea.Cmd {
	return a.m.Init()
}

func (a *teaModelAdapter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m2, cmd := Update(a.m, msg)
	a.m = m2
	return a, cmd
}

func (a *teaModelAdapter) View() string {
	return ModelView(a.m)
}