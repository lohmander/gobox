package task

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Position represents a range within the task or markdown document, identified by start and end indexes.
type Position struct {
	Start int
	End   int
}

// Task represents a task parsed from the Markdown file.
type Task struct {
	Description string // The text of the task description
	TimeBox     string // The raw timebox string, e.g., "@1h", "@[10:00-13:00]"
	IsChecked   bool   // True if the task is already checked
	Position    Position
}

// Hash generates a unique hash for the task based on its Description and TimeBox.
func (t *Task) Hash() string {
	data := fmt.Sprintf("%s|%s", t.Description, t.TimeBox)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// String returns a markdown task list item representation of the task with its description, time box, and checked status.
func (t *Task) String() string {
	checkMark := " "

	if t.IsChecked {
		checkMark = "x"
	}

	return fmt.Sprintf("- [%s] %s %s", checkMark, t.Description, t.TimeBox)
}
