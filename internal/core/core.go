package core

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"gobox/internal/clock"   // Import clock abstraction
	"gobox/internal/gitutil" // Import gitutil
	"gobox/internal/parser"  // Import parser
	"gobox/internal/state"   // Import state for timebox state management
	"gobox/pkg/task"         // Import task
)

// Global variables for managing the Git watcher and terminal output.
// These are now private to the 'core' package.
var (
	// mu protects access to lastPrintedCommitHashes to prevent race conditions
	mu                       sync.Mutex
	lastPrintedCommitHashes  = make(map[string]struct{}) // Set of commit hashes already printed
	terminalOutputMutex      sync.Mutex                  // Protects terminal output to prevent garbling
	currentTimerLineLength   int                         // Stores the length of the last printed timer line
	currentCommitDisplayLine int                         // Stores the line number where commits are displayed
)

// clearLine clears the current line in the terminal.
func clearLine() {
	fmt.Print("\r" + strings.Repeat(" ", currentTimerLineLength) + "\r")
}

// printTimerStatus updates the timer display on the current line.
func printTimerStatus(message string) {
	terminalOutputMutex.Lock()
	defer terminalOutputMutex.Unlock()

	clearLine() // Clear the previous timer line
	fmt.Print(message)
	currentTimerLineLength = len(message) // Update the length for the next clear
}

// printCommit prints a new commit message, ensuring it doesn't interfere with the timer.
func printCommit(commit string) {
	terminalOutputMutex.Lock()
	defer terminalOutputMutex.Unlock()

	// Move cursor to the line below the timer, clear it, print commit, then move back up
	// This is a basic approach; for more complex UIs, a library like 'termbox-go' would be better.
	fmt.Printf("\n%s\r", commit) // Print on a new line
	currentCommitDisplayLine++
}

// timerAndGitWatcher manages the countdown and real-time commit display, using a Clock for testability.
func timerAndGitWatcher(
	taskDesc string,
	duration time.Duration,
	endTime time.Time,
	startTime time.Time,
	stopChan chan struct{},
	wg *sync.WaitGroup,
	clk clock.Clock,
) {
	defer wg.Done()

	ticker := clk.NewTicker(1 * time.Second)
	defer ticker.Stop()

	gitPollTicker := clk.NewTicker(5 * time.Second) // Poll Git every 5 seconds
	defer gitPollTicker.Stop()

	// Initial print of the task
	fmt.Printf("Starting task: %s\n", taskDesc)
	fmt.Println("Press Enter to finish early.")

	lastPrintedCommitHashes = make(map[string]struct{}) // Reset for new task

	for {
		select {
		case <-stopChan:
			// Ensure the last timer message is cleared before exiting
			printTimerStatus("Task finished!")
			fmt.Println() // Move to a new line after final status
			return
		case <-ticker.C():
			// Update timer display
			var remaining time.Duration
			if duration > 0 { // Duration-based timer
				elapsed := clk.Now().Sub(startTime)
				remaining = duration - elapsed
				if remaining <= 0 {
					select {
					case stopChan <- struct{}{}: // Signal main to stop
					default:
					}
					return
				}
				printTimerStatus(fmt.Sprintf("Time remaining: %s", remaining.Round(time.Second)))
			} else if !endTime.IsZero() { // Time-range based timer
				remaining = endTime.Sub(clk.Now())
				if remaining <= 0 {
					select {
					case stopChan <- struct{}{}: // Signal main to stop
					default:
					}
					return
				}
				printTimerStatus(fmt.Sprintf("Ends at: %s (Remaining: %s)", endTime.Format("15:04:05"), remaining.Round(time.Second)))
			}

		case <-gitPollTicker.C():
			// Poll for new commits
			commits, err := gitutil.GetCommitsSince(startTime) // Use gitutil package
			if err != nil {
				// Only print error if it's not "not a git repository"
				if !strings.Contains(err.Error(), "not a git repository") {
					printCommit(fmt.Sprintf("Error fetching commits: %v", err))
				}
				continue
			}

			mu.Lock()
			for _, commit := range commits {
				// Extract commit hash (first 7-8 chars)
				parts := strings.SplitN(commit, " ", 2)
				if len(parts) > 0 {
					hash := parts[0]
					if _, found := lastPrintedCommitHashes[hash]; !found {
						printCommit(fmt.Sprintf("New commit: %s", commit))
						lastPrintedCommitHashes[hash] = struct{}{}
					}
				}
			}
			mu.Unlock()
		}
	}
}

// StartGoBox is the main entry point for the GoBox application logic.
// It returns an error or nil. The CLI should handle os.Exit.
// Accepts an optional clock.Clock for testability; uses RealClock if nil.
func StartGoBox(markdownFile string) error {
	return StartGoBoxWithClockAndStore(markdownFile, clock.RealClock{}, NewFileStateStore(".gobox_state.json"))
}

// StartGoBoxWithClockAndStore allows injecting both a clock and a StateStore for testability.
func StartGoBoxWithClockAndStore(markdownFile string, clk clock.Clock, stateMgr StateStore) error {
	if clk == nil {
		clk = clock.RealClock{}
	}
	tasks, err := parser.ParseMarkdownFile(markdownFile)
	if err != nil {
		return fmt.Errorf("Error parsing markdown file: %w", err)
	}

	nextTask := selectNextTask(tasks)
	if nextTask == nil {
		fmt.Println("No unchecked tasks with time boxes found in the markdown file.")
		return nil
	}

	duration, endTime, err := parser.ParseTimeBox(nextTask.TimeBox)
	if err != nil {
		return fmt.Errorf("Error parsing time box '%s': %v", nextTask.TimeBox, err)
	}

	actualDuration, actualEndTime, skip := determineTimer(duration, endTime, nextTask)
	if skip {
		return nil
	}

	states, _ := stateMgr.Load()
	taskHash := nextTask.Hash()
	now := clk.Now()
	states, currentState := findOrCreateState(states, taskHash, now)
	stateMgr.Save(states)

	elapsed, timerStartTime := calculateElapsedAndStart(currentState, now)
	setupSignalHandler(states, stateMgr, taskHash)

	stopChan := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	timerDuration := getTimerDuration(actualDuration, elapsed, nextTask, stateMgr, states)
	go timerAndGitWatcher(nextTask.Description, timerDuration, actualEndTime, timerStartTime, stopChan, &wg, clk)

	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadBytes('\n')
	select {
	case stopChan <- struct{}{}:
	default:
	}
	wg.Wait()

	finalEndTime := clk.Now()
	closeCurrentSegmentIfOpen(currentState, finalEndTime, stateMgr, states)
	commitsDuringTask := getCommitsDuringTask(timerStartTime)
	nextTask.IsChecked = true
	totalDuration := calculateTotalDuration(currentState, finalEndTime)
	err = parser.UpdateMarkdown(markdownFile, *nextTask, commitsDuringTask, totalDuration)
	newStates := stateMgr.RemoveTaskState(states, taskHash)
	stateMgr.Save(newStates)

	if err != nil {
		return fmt.Errorf("Error updating markdown file: %v", err)
	}

	fmt.Println("\nTask completed and markdown updated!")
	return nil
}

// For backward compatibility, keep StartGoBoxWithClock as a wrapper.
func StartGoBoxWithClock(markdownFile string, clk clock.Clock) error {
	return StartGoBoxWithClockAndStore(markdownFile, clk, NewFileStateStore(".gobox_state.json"))
}

// CompleteTask marks a task as checked, updates the markdown file, and records duration/commits.
// It sums all segments in the TimeBoxState for total duration.
func CompleteTask(markdownFile string, t task.Task, tbState state.TimeBoxState, commits []string) error {
	updated := t
	updated.IsChecked = true
	var totalDuration time.Duration
	for _, seg := range tbState.Segments {
		if seg.End != nil {
			totalDuration += seg.End.Sub(seg.Start)
		}
	}
	return parser.UpdateMarkdown(markdownFile, updated, commits, totalDuration)
}

// --- Helper Functions ---

func selectNextTask(tasks []task.Task) *task.Task {
	for i := range tasks {
		if !tasks[i].IsChecked && tasks[i].TimeBox != "" {
			return &tasks[i]
		}
	}
	return nil
}

func determineTimer(duration time.Duration, endTime time.Time, nextTask *task.Task) (time.Duration, time.Time, bool) {
	if duration > 0 {
		return duration, time.Time{}, false
	} else if !endTime.IsZero() {
		if time.Until(endTime) <= 0 {
			fmt.Printf("Task '%s' with timebox '%s' is already past its end time. Skipping.\n", nextTask.Description, nextTask.TimeBox)
			return 0, time.Time{}, true
		}
		return 0, endTime, false
	}
	fmt.Printf("Task '%s' has an invalid or unsupported timebox: %s\n", nextTask.Description, nextTask.TimeBox)
	return 0, time.Time{}, true
}

func findOrCreateState(states []state.TimeBoxState, taskHash string, now time.Time) ([]state.TimeBoxState, *state.TimeBoxState) {
	for i := range states {
		if states[i].TaskHash == taskHash {
			if len(states[i].Segments) == 0 || states[i].Segments[len(states[i].Segments)-1].End != nil {
				states[i].Segments = append(states[i].Segments, state.TimeSegment{Start: now, End: nil})
			}
			return states, &states[i]
		}
	}
	states = append(states, state.TimeBoxState{
		TaskHash: taskHash,
		Segments: []state.TimeSegment{{Start: now, End: nil}},
	})
	return states, &states[len(states)-1]
}

func calculateElapsedAndStart(currentState *state.TimeBoxState, now time.Time) (time.Duration, time.Time) {
	var elapsed time.Duration
	for _, seg := range currentState.Segments {
		if seg.End != nil {
			elapsed += seg.End.Sub(seg.Start)
		} else {
			elapsed += now.Sub(seg.Start)
		}
	}
	var timerStartTime time.Time
	if len(currentState.Segments) > 0 && currentState.Segments[len(currentState.Segments)-1].End == nil {
		timerStartTime = currentState.Segments[len(currentState.Segments)-1].Start
	} else {
		timerStartTime = now
	}
	return elapsed, timerStartTime
}

func setupSignalHandler(states []state.TimeBoxState, stateMgr StateStore, taskHash string) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		fmt.Printf("\nReceived signal: %v. Pausing timebox and saving state...\n", sig)
		now := time.Now()
		for i := range states {
			if states[i].TaskHash == taskHash && len(states[i].Segments) > 0 {
				lastSeg := &states[i].Segments[len(states[i].Segments)-1]
				if lastSeg.End == nil {
					lastSeg.End = &now
				}
			}
		}
		stateMgr.Save(states)
		os.Exit(130)
	}()
}

func getTimerDuration(actualDuration, elapsed time.Duration, nextTask *task.Task, stateMgr StateStore, states []state.TimeBoxState) time.Duration {
	if actualDuration > 0 {
		if elapsed >= actualDuration {
			fmt.Println("Task has already used up its allocated timebox. Marking as done.")
			nextTask.IsChecked = true
			stateMgr.Save(states)
			return 0
		}
		return actualDuration - elapsed
	}
	return 0
}

func closeCurrentSegmentIfOpen(currentState *state.TimeBoxState, finalEndTime time.Time, stateMgr StateStore, states []state.TimeBoxState) {
	if currentState != nil && len(currentState.Segments) > 0 {
		lastSeg := &currentState.Segments[len(currentState.Segments)-1]
		if lastSeg.End == nil {
			lastSeg.End = &finalEndTime
			stateMgr.Save(states)
		}
	}
}

func getCommitsDuringTask(timerStartTime time.Time) []string {
	allCommits, err := gitutil.GetCommitsSince(timerStartTime)
	if err != nil {
		fmt.Printf("Warning: Could not fetch Git commits: %v\n", err)
		allCommits = []string{}
	}
	var commitsDuringTask []string
	for _, commitLine := range allCommits {
		commitsDuringTask = append(commitsDuringTask, commitLine)
	}
	return commitsDuringTask
}

func calculateTotalDuration(currentState *state.TimeBoxState, finalEndTime time.Time) time.Duration {
	var totalDuration time.Duration
	for _, seg := range currentState.Segments {
		if seg.End != nil {
			totalDuration += seg.End.Sub(seg.Start)
		} else {
			totalDuration += finalEndTime.Sub(seg.Start)
		}
	}
	return totalDuration
}
