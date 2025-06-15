package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ModelView renders the TUI model's view as a string.
func ModelView(m model) string {
	switch m.ActiveView {
	case ViewQuitting:
		return quittingView()
	case ViewTimerActive:
		return timerView(m)
	case ViewTimerDone:
		return completionView()
	case ViewTaskList:
		return taskListView(m)
	default:
		return taskListView(m)
	}
}

func quittingView() string {
	return "Goodbye!\n"
}

func timerView(m model) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FFFF"))

	timeStr := m.timer.Round(1e9).String()  // time.Second
	timerColor := lipgloss.Color("#00FF00") // green for normal time

	// Change timer color to yellow when less than 20% time remains
	if m.timerTotal > 0 && m.timer < m.timerTotal/5 {
		timerColor = lipgloss.Color("#FFFF00")
	}

	// Change timer color to red when less than 10% time remains
	if m.timerTotal > 0 && m.timer < m.timerTotal/10 {
		timerColor = lipgloss.Color("#FF0000")
	}

	timerStyle := lipgloss.NewStyle().Foreground(timerColor).Bold(true)

	timerBlock := lipgloss.NewStyle().Padding(1).BorderStyle(lipgloss.RoundedBorder()).Render(
		fmt.Sprintf(
			"%s\n%s\n\nPress Enter to complete early or q/Ctrl+C to quit.",
			headerStyle.Render("Working on: ")+m.TimerTask.Title(),
			headerStyle.Render("Time remaining: ")+timerStyle.Render(timeStr),
		),
	)
	commitsBlock := lipgloss.NewStyle().Padding(1).Render(headerStyle.Render("Commits during session:"))

	// Only render commit table if it has columns and rows
	commitTableBlock := ""
	if len(m.commitTable.Columns()) > 0 {
		commitTableBlock = m.commitTable.View()
	}

	content := lipgloss.JoinVertical(lipgloss.Left, timerBlock, commitsBlock, commitTableBlock)
	contentLines := strings.Count(content, "\n") + 1
	if m.height > contentLines {
		content += strings.Repeat("\n", m.height-contentLines)
	}
	return content
}

func completionView() string {
	// Show completion message and return to list after a keypress
	successStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	instructionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))
	return lipgloss.NewStyle().Padding(1).BorderStyle(lipgloss.DoubleBorder()).Render(
		fmt.Sprintf("%s\n\n%s",
			successStyle.Render("✅ Task completed successfully!"),
			instructionStyle.Render("Press Enter or Space to mark as complete and return to the list.")),
	)
}

func taskListView(m model) string {
	taskList := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#555555")).
		Padding(1).
		Render(m.list.View())

	return lipgloss.JoinVertical(lipgloss.Left,
		taskList,
	)
}
