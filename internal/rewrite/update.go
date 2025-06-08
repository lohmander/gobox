package rewrite

import (
	"bufio"
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
	file, err := os.Open(markdownFile)
	if err != nil {
		return fmt.Errorf("failed to open markdown file: %w", err)
	}
	defer file.Close()

	// Read file line by line to find and replace task
	scanner := bufio.NewScanner(file)
	var lines []string
	taskFound := false

	for scanner.Scan() {
		line := scanner.Text()
		
		// Check if this line is our task
		if !taskFound && isTaskLine(line, taskDesc) {
			// Convert "[ ]" to "[x]" to mark as completed
			line = strings.Replace(line, "[ ]", "[x]", 1)
			taskFound = true
			
			// Add duration info
			if totalDuration > 0 {
				hours := int(totalDuration.Hours())
				minutes := int(totalDuration.Minutes()) % 60
				seconds := int(totalDuration.Seconds()) % 60
				line = fmt.Sprintf("%s  ⏱️ %dh %dm %ds", line, hours, minutes, seconds)
			}
			
			lines = append(lines, line)
			
			// Add commit information if available
			if len(commits) > 0 {
				lines = append(lines, "")
				lines = append(lines, "  Commits:")
				for _, commit := range commits {
					lines = append(lines, fmt.Sprintf("  - %s", commit))
				}
			}
		} else {
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading markdown file: %w", err)
	}

	if !taskFound {
		return fmt.Errorf("task not found in markdown file: %s", taskDesc)
	}

	// Write the modified content back to the file
	err = os.WriteFile(markdownFile, []byte(strings.Join(lines, "\n")), 0644)
	if err != nil {
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