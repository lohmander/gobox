package rewrite

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gobox/pkg/task"
)

// MarkTaskAsCompleted marks a specific task as completed in a markdown file
// and adds information about time spent and commits.
func MarkTaskAsCompleted(
	markdownFile string,
	taskDesc string,
	totalDuration time.Duration,
	commits []string,
) error {
	// Read the file
	content, err := os.ReadFile(markdownFile)
	if err != nil {
		return fmt.Errorf("failed to open markdown file: %w", err)
	}

	// Create a rewriter to modify the content
	lineOffsets := BuildLineOffsets(content)
	rewriter := NewScannerRewriter(bytes.NewReader(content), lineOffsets)

	// Find the task line
	lines := bytes.Split(content, []byte("\n"))
	taskLineIndex := -1
	for i, line := range lines {
		if isTaskLine(string(line), taskDesc) {
			taskLineIndex = i
			break
		}
	}

	if taskLineIndex == -1 {
		return fmt.Errorf("task not found in markdown file: %s", taskDesc)
	}

	// Copy the content up to the task line
	if err := rewriter.CopyLinesUntil(taskLineIndex); err != nil {
		return fmt.Errorf("error copying lines: %w", err)
	}

	// Create the modified line content
	var newLines [][]byte

	// Update the task line to mark it as completed
	taskLine := string(lines[taskLineIndex])
	taskLine = strings.Replace(taskLine, "[ ]", "[x]", 1)

	// Add duration information if available
	if totalDuration > 0 {
		hours := int(totalDuration.Hours())
		minutes := int(totalDuration.Minutes()) % 60
		seconds := int(totalDuration.Seconds()) % 60
		taskLine = fmt.Sprintf("%s\n\n  ⏱️ %dh %dm %ds  ", taskLine, hours, minutes, seconds)
	}

	newLines = append(newLines, []byte(taskLine))

	// Add commit information if available
	if len(commits) > 0 {
		newLines = append(newLines, []byte("  📝 Commits:"))
		for _, commit := range commits {
			newLines = append(newLines, fmt.Appendf(nil, "  - %s", commit))
		}
	}

	// Replace the original task line with our new content
	if err := rewriter.ReplaceLines(taskLineIndex, taskLineIndex, newLines); err != nil {
		return fmt.Errorf("error replacing lines: %w", err)
	}

	// Copy any remaining lines
	if err := rewriter.CopyRemainingLines(); err != nil {
		return fmt.Errorf("error copying remaining lines: %w", err)
	}

	// Write the modified content back to the file
	if err := os.WriteFile(markdownFile, rewriter.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write updated markdown file: %w", err)
	}

	return nil
}

// isTaskLine checks if a line contains an unchecked task with the given description
func isTaskLine(line string, taskDesc string) bool {
	// Regexp for markdown task list item: "- [ ] Task description"
	taskPattern := regexp.MustCompile(`^\s*-\s*\[\s*\]\s+(.+)$`)

	matches := taskPattern.FindStringSubmatch(line)
	if len(matches) < 2 {
		return false
	}

	// Extract task description from the line and trim any timebox annotation
	description := matches[1]

	// Remove timebox notation if present (e.g., "@1h", "@30m", "@[10:00-11:00]")
	timeboxPattern := regexp.MustCompile(`\s*@(\d+h\d+m|\d+h|\d+m|\[\d+:\d+-\d+:\d+\])\s*$`)
	description = timeboxPattern.ReplaceAllString(description, "")
	description = strings.TrimSpace(description)

	// Compare with the target task description
	return strings.EqualFold(description, strings.TrimSpace(taskDesc))
}

// UpdateTaskWithState updates a task in the markdown file based on its description
// and records the time spent from the state.
func UpdateTaskWithState(
	markdownFile string,
	t task.Task,
	totalDuration time.Duration,
	commits []string,
) error {
	// Just use MarkTaskAsCompleted as it covers all our needs
	return MarkTaskAsCompleted(markdownFile, t.Description, totalDuration, commits)
}
