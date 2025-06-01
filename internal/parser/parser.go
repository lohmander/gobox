package parser

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gobox/pkg/task"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// ParseMarkdownFile reads the markdown file and extracts tasks with time boxes.
func ParseMarkdownFile(filename string) ([]task.Task, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	// Create a Goldmark Markdown parser
	md := goldmark.New(goldmark.WithExtensions(extension.TaskList))
	reader := text.NewReader(content)
	rootNode := md.Parser().Parse(reader)

	var tasks []task.Task
	// lineNumber := 0

	// Regex to match checklist items and optionally capture timebox syntax
	re := regexp.MustCompile(`(@(\d+[mh]?|\d+h\d+m|\[\d{2}:\d{2}-\d{2}:\d{2}\]))?\s*$`)

	// Traverse the AST to find list items
	ast.Walk(rootNode, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if check, ok := node.(*east.TaskCheckBox); ok {
			if text, ok := node.NextSibling().(*ast.Text); ok {
				itemText := strings.TrimSpace(string(text.Value(content)))
				matches := re.FindSubmatch(text.Value(content))
				timeBox := strings.TrimSpace(string(matches[0]))

				tasks = append(tasks, task.Task{
					Description: itemText,
					TimeBox:     timeBox,
				})
				fmt.Printf("Node type: %T <-> %T: %s, %s\n", check, text, string(text.Value(content)), matches[0])
			}
		}

		return ast.WalkContinue, nil
	})

	return tasks, nil
}

// extractText extracts the plain text content from a Markdown node.
func extractText(node ast.Node, content []byte) string {
	var buf bytes.Buffer
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if textNode, ok := n.(*ast.Text); ok && entering {
			buf.Write(textNode.Segment.Value(content))
		}
		return ast.WalkContinue, nil
	})
	return buf.String()
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

// UpdateMarkdown updates the task, adds commits, and records actual time spent in the markdown file.
func UpdateMarkdown(filename string, task task.Task, commits []string, startTime, endTime time.Time) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	// Create a Goldmark Markdown parser
	md := goldmark.New()
	reader := text.NewReader(content)
	rootNode := md.Parser().Parse(reader)

	var buffer bytes.Buffer
	lineNumber := 0

	// Traverse the AST to find and update the target task
	ast.Walk(rootNode, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if listItem, ok := node.(*ast.ListItem); ok {
			lineNumber++
			text := extractText(listItem, content)

			if text == task.OriginalLine {
				// Update the task line to be checked
				updatedLine := strings.Replace(text, "- [ ]", "- [x]", 1)
				buffer.WriteString(updatedLine + "\n")

				// Add time tracking and commit lines
				if !startTime.IsZero() && !endTime.IsZero() {
					duration := endTime.Sub(startTime)
					buffer.WriteString(fmt.Sprintf("    * Completed: %s\n", endTime.Format("2006-01-02 15:04 MST")))
					buffer.WriteString(fmt.Sprintf("    * Duration: %s\n", duration.Round(time.Second).String()))
				}
				if len(commits) > 0 {
					buffer.WriteString("    * Commits during task:\n")
					for _, commit := range commits {
						buffer.WriteString(fmt.Sprintf("        - %s\n", commit))
					}
				}
				return ast.WalkSkipChildren, nil
			}
		}

		// Render the original content for other nodes
		if entering {
			buffer.Write(content[node.Lines().At(0).Start:node.Lines().At(0).Stop])
		}
		return ast.WalkContinue, nil
	})

	// Write the updated content back to the file
	return os.WriteFile(filename, buffer.Bytes(), 0644)
}
