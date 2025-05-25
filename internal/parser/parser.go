package parser

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gobox/pkg/task" // Import the Task struct
)

// ParseMarkdownFile reads the markdown file and extracts tasks with time boxes.
// This function uses a line-by-line regex approach to reliably capture line numbers
// and original line content, which is essential for updating the markdown file.
func ParseMarkdownFile(filename string) ([]task.Task, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer file.Close()

	var tasks []task.Task
	scanner := bufio.NewScanner(file)
	lineNumber := 0

	// Regex to match checklist items and optionally capture timebox syntax.
	// Group 1: ' ' or 'x' (for checked/unchecked)
	// Group 2: Task description
	// Group 3: Full timebox string (e.g., "@1h", "@[10:00-13:00]")
	// Group 4: Timebox content (e.g., "1h", "10:00-13:00")
	re := regexp.MustCompile(`^- \[( |x)\]\s*(.*?)\s*(@(\d+[mh]?|\d+h\d+m|\[\d{2}:\d{2}-\d{2}:\d{2}\]))?\s*$`)

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)

		if len(matches) > 0 {
			isChecked := matches[1] == "x"
			description := strings.TrimSpace(matches[2])
			timeBox := strings.TrimSpace(matches[3]) // This will be empty if no timebox is present

			tasks = append(tasks, task.Task{
				Description:  description,
				TimeBox:      timeBox,
				LineNumber:   lineNumber,
				OriginalLine: line, // Directly capture the original line
				IsChecked:    isChecked,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file %s: %w", filename, err)
	}

	return tasks, nil
}

// ParseTimeBox parses the timebox string into a duration or an end time.
// It returns duration, endTime, error.
// If duration is non-zero, it's a duration-based box.
// If endTime is non-zero, it's a time-range-based box.
func ParseTimeBox(timeBox string) (time.Duration, time.Time, error) {
	if timeBox == "" {
		return 0, time.Time{}, fmt.Errorf("no timebox provided")
	}

	// Remove the leading '@' if present
	if strings.HasPrefix(timeBox, "@") {
		timeBox = timeBox[1:]
	}

	// Check for time range syntax: [HH:MM-HH:MM]
	if strings.HasPrefix(timeBox, "[") && strings.HasSuffix(timeBox, "]") {
		timeRangeStr := strings.Trim(timeBox, "[]")
		parts := strings.Split(timeRangeStr, "-")
		if len(parts) != 2 {
			return 0, time.Time{}, fmt.Errorf("invalid time range format: %s. Expected [HH:MM-HH:MM]", timeBox)
		}

		endStr := strings.TrimSpace(parts[1])

		now := time.Now()
		endTime, err := time.Parse("15:04", endStr)
		if err != nil {
			return 0, time.Time{}, fmt.Errorf("invalid end time format in %s: %w", timeBox, err)
		}
		// Set the year, month, day to today's date
		endTime = time.Date(now.Year(), now.Month(), now.Day(), endTime.Hour(), endTime.Minute(), 0, 0, now.Location())

		// If the end time is in the past (e.g., 10:00 AM when it's 2:00 PM),
		// assume it's for the next day.
		if endTime.Before(now) {
			endTime = endTime.Add(24 * time.Hour)
		}

		return 0, endTime, nil // Return 0 duration, valid end time
	}

	// Assume duration syntax: 1h, 30m, 1h30m
	var totalDuration time.Duration
	reDuration := regexp.MustCompile(`^(\d+h)?(\d+m)?$`)
	matches := reDuration.FindStringSubmatch(timeBox)

	if len(matches) == 3 {
		if matches[1] != "" { // hours part
			hours, err := strconv.Atoi(strings.TrimSuffix(matches[1], "h"))
			if err != nil {
				return 0, time.Time{}, fmt.Errorf("invalid hours in duration %s: %w", timeBox, err)
			}
			totalDuration += time.Duration(hours) * time.Hour
		}
		if matches[2] != "" { // minutes part
			minutes, err := strconv.Atoi(strings.TrimSuffix(matches[2], "m"))
			if err != nil {
				return 0, time.Time{}, fmt.Errorf("invalid minutes in duration %s: %w", timeBox, err)
			}
			totalDuration += time.Duration(minutes) * time.Minute
		}
		if totalDuration > 0 {
			return totalDuration, time.Time{}, nil // Return valid duration, 0 end time
		}
	}

	return 0, time.Time{}, fmt.Errorf("unsupported timebox format: %s. Expected @1h, @30m, @1h30m or @[HH:MM-HH:MM]", timeBox)
}

// UpdateMarkdown checks the task, adds commits, and records actual time spent to the markdown file.
func UpdateMarkdown(filename string, task task.Task, commits []string, startTime, endTime time.Time) error {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	lines := strings.Split(string(content), "\n")
	if task.LineNumber <= 0 || task.LineNumber > len(lines) {
		return fmt.Errorf("invalid line number %d for task '%s'", task.LineNumber, task.Description)
	}

	// Update the task line to be checked
	originalLine := lines[task.LineNumber-1]
	updatedLine := strings.Replace(originalLine, "- [ ]", "- [x]", 1)
	lines[task.LineNumber-1] = updatedLine

	// Prepare time tracking lines
	var timeLines []string
	if !startTime.IsZero() && !endTime.IsZero() {
		duration := endTime.Sub(startTime)
		timeLines = append(timeLines, fmt.Sprintf("    * Completed: %s", endTime.Format("2006-01-02 15:04 MST")))
		timeLines = append(timeLines, fmt.Sprintf("    * Duration: %s", duration.Round(time.Second).String()))
	}

	// Prepare commits to be inserted
	var commitLines []string
	if len(commits) > 0 {
		commitLines = append(commitLines, "    * Commits during task:")
		for _, commit := range commits {
			commitLines = append(commitLines, fmt.Sprintf("        - %s", commit))
		}
	}

	// Insert time and commit lines right after the updated task line
	newLines := make([]string, 0, len(lines)+len(timeLines)+len(commitLines))
	newLines = append(newLines, lines[:task.LineNumber]...)
	newLines = append(newLines, timeLines...)
	newLines = append(newLines, commitLines...)
	newLines = append(newLines, lines[task.LineNumber:]...)

	newContent := strings.Join(newLines, "\n")
	return ioutil.WriteFile(filename, []byte(newContent), 0644)
}
