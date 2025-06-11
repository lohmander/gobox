package parser

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gobox/internal/rewrite"
	"gobox/pkg/task"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

func extractTextSkippingNode(n ast.Node, skip ast.Node, content []byte, builder *strings.Builder) {
	if n == skip {
		return
	}

	if t, ok := n.(*ast.Text); ok {
		builder.Write(t.Segment.Value(content))
	} else {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			extractTextSkippingNode(c, skip, content, builder)
		}
	}
}

func ExtractTask(node ast.Node, content []byte) (*task.Task, bool) {
	re := regexp.MustCompile(`(@(?:\d+h\d+m|\d+h|\d+m))(?:\s|$)`)

	if check, ok := node.(*east.TaskCheckBox); ok {
		listItem := FindParentListItem(check)
		if listItem == nil {
			return nil, false
		}

		var descBuilder strings.Builder

		// Extract text from all children of the list item, skipping the checkbox
		for c := listItem.FirstChild(); c != nil; c = c.NextSibling() {
			extractTextSkippingNode(c, check, content, &descBuilder)
		}

		descText := strings.TrimSpace(descBuilder.String())

		matches := re.FindSubmatch([]byte(descText))
		timeBox := ""

		if len(matches) > 1 {
			timeBox = string(matches[1]) // the full `@25m` or `@[10:00-11:00]`
		}

		itemText := strings.TrimSuffix(descText, timeBox)
		itemText = strings.TrimSpace(itemText)

		return &task.Task{
			Description: itemText,
			TimeBox:     timeBox,
			IsChecked:   check.IsChecked,
		}, true
	}

	return nil, false
}

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

	tasks := []task.Task{}

	// Traverse the AST to find list items
	ast.Walk(rootNode, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if task, ok := ExtractTask(node, content); ok {
			tasks = append(tasks, *task)
		}

		return ast.WalkContinue, nil
	})

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

// UpdateMarkdown updates the task, adds commits, and records actual time spent in the markdown file.
// totalDuration should be the sum of all time segments for the task.
func UpdateMarkdown(
	filename string,
	updatedTask task.Task,
	commits []string,
	totalDuration time.Duration,
) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	// Parse into AST
	md := goldmark.New(goldmark.WithExtensions(extension.TaskList))
	reader := text.NewReader(content)
	rootNode := md.Parser().Parse(reader)

	// Create a new scanner rewriter to modify the content
	rewriter := rewrite.NewScannerRewriter(
		bytes.NewReader(content),
		rewrite.BuildLineOffsets(content),
	)

	err = ast.Walk(rootNode, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			return ast.WalkContinue, nil
		}

		if parsedTask, ok := ExtractTask(n, content); ok {
			if parsedTask.Hash() == updatedTask.Hash() {
				p := FindParentListItem(n)
				prev := p.FirstChild().Lines().At(0)
				startIndex := rewriter.LineIndexOfByte(prev.Start)
				endIndex := rewriter.LineIndexOfByte(prev.Stop)

				var taskText [][]byte

				taskText = append(taskText, []byte(updatedTask.String()))

				// Add actual duration if totalDuration is set
				if totalDuration > 0 {
					hours := int(totalDuration.Hours())
					minutes := int(totalDuration.Minutes()) % 60
					seconds := int(totalDuration.Seconds()) % 60
					durationStr := fmt.Sprintf("\n⏱️ %dh %dm %ds", hours, minutes, seconds)
					taskText = append(taskText, []byte(durationStr))
				}

				rewriter.CopyLinesUntil(startIndex)

				// Replace the task item with the updated task
				rewriter.ReplaceLines(startIndex, endIndex, taskText)
			}
		}

		return ast.WalkContinue, nil
	})

	if err := rewriter.CopyRemainingLines(); err != nil {
		return fmt.Errorf("failed to copy remaining lines: %w", err)
	}

	// Finally, write out the buffer
	return os.WriteFile(filename, rewriter.Bytes(), 0644)
}

func FindParentListItem(n ast.Node) ast.Node {
	if parent := n.Parent(); parent != nil {
		if parent.Kind() == ast.KindListItem {
			return parent
		}
		return FindParentListItem(parent)
	}
	return nil
}
