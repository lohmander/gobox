package task

// Task represents a task parsed from the Markdown file.
type Task struct {
	Description  string
	TimeBox      string // The raw timebox string, e.g., "@1h", "@[10:00-13:00]"
	LineNumber   int    // 1-based line number in the original file
	OriginalLine string // The full original line content
	IsChecked    bool   // True if the task is already checked
}
