package tui

import (
	"fmt"
	"strings"

	"gobox/internal/core"
	"gobox/internal/parser"
	"gobox/internal/state"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

// wrapText wraps input text to lines no longer than maxWidth display cells.
// It wraps on word boundaries to avoid breaking words when possible.
func wrapText(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}

	var lines []string
	for _, paragraph := range strings.Split(s, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			// Empty paragraph, add empty line
			lines = append(lines, "")
			continue
		}

		var lineBuilder strings.Builder
		lineWidth := 0
		spaceWidth := runewidth.StringWidth(" ")
		for i, word := range words {
			wordWidth := runewidth.StringWidth(word)
			// Calculate if adding the word would exceed maxWidth
			// Add a space before word if not first word in line
			addedWidth := wordWidth
			if lineWidth > 0 {
				addedWidth += spaceWidth
			}
			if lineWidth+addedWidth > maxWidth {
				// start new line
				lines = append(lines, lineBuilder.String())
				lineBuilder.Reset()
				lineBuilder.WriteString(word)
				lineWidth = wordWidth
			} else {
				if lineWidth > 0 {
					lineBuilder.WriteString(" ")
					lineWidth += spaceWidth
				}
				lineBuilder.WriteString(word)
				lineWidth += wordWidth
			}
			// If last word, append line
			if i == len(words)-1 {
				lines = append(lines, lineBuilder.String())
			}
		}
	}
	return strings.Join(lines, "\n")
}

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

		tasks = append(tasks, TaskItem{RawLine: line, Task: t})
	}

	m := InitialModel(tasks, markdownFile, 24, stateMgr, states)
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
