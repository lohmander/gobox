package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

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

// timerAndGitWatcher manages the countdown and real-time commit display.
func timerAndGitWatcher(taskDesc string, duration time.Duration, endTime time.Time, startTime time.Time, stopChan chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	gitPollTicker := time.NewTicker(5 * time.Second) // Poll Git every 5 seconds
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
		case <-ticker.C:
			// Update timer display
			var remaining time.Duration
			if duration > 0 { // Duration-based timer
				elapsed := time.Since(startTime)
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
				remaining = time.Until(endTime)
				if remaining <= 0 {
					select {
					case stopChan <- struct{}{}: // Signal main to stop
					default:
					}
					return
				}
				printTimerStatus(fmt.Sprintf("Ends at: %s (Remaining: %s)", endTime.Format("15:04:05"), remaining.Round(time.Second)))
			}

		case <-gitPollTicker.C:
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
func StartGoBox(markdownFile string) {
	tasks, err := parser.ParseMarkdownFile(markdownFile) // Use parser package
	if err != nil {
		fmt.Printf("Error parsing markdown file: %v\n", err)
		os.Exit(1)
	}

	var nextTask *task.Task // Use task package
	for i := range tasks {
		if !tasks[i].IsChecked && tasks[i].TimeBox != "" {
			nextTask = &tasks[i]
			break
		}
	}

	if nextTask == nil {
		fmt.Println("No unchecked tasks with time boxes found in the markdown file.")
		os.Exit(0)
	}

	duration, endTime, err := parser.ParseTimeBox(nextTask.TimeBox) // Use parser package
	if err != nil {
		fmt.Printf("Error parsing time box '%s': %v\n", nextTask.TimeBox, err)
		os.Exit(1)
	}

	// Determine effective duration/end time for the timer
	var actualDuration time.Duration
	var actualEndTime time.Time
	if duration > 0 {
		actualDuration = duration
	} else if !endTime.IsZero() {
		actualEndTime = endTime
		// If the end time is in the past, consider the task already done or for next day
		if time.Until(actualEndTime) <= 0 {
			fmt.Printf("Task '%s' with timebox '%s' is already past its end time. Skipping.\n", nextTask.Description, nextTask.TimeBox)
			os.Exit(0)
		}
	} else {
		fmt.Printf("Task '%s' has an invalid or unsupported timebox: %s\n", nextTask.Description, nextTask.TimeBox)
		os.Exit(1)
	}

	startTime := time.Now()

	// --- STATE MANAGEMENT: Save state when a task begins ---
	stateFile := ".gobox_state.json"
	var states []state.TimeBoxState

	// Try to read the state file if it exists
	if f, err := os.Open(stateFile); err == nil {
		defer f.Close()
		dec := json.NewDecoder(f)
		if err := dec.Decode(&states); err != nil && err.Error() != "EOF" {
			fmt.Printf("Warning: Could not decode state file: %v\n", err)
		}
	}

	// Find or create the TimeBoxState for the current task
	taskHash := nextTask.Hash()
	found := false
	for i := range states {
		if states[i].TaskHash == taskHash {
			// Add a new segment for this session
			states[i].Segments = append(states[i].Segments, state.TimeSegment{Start: startTime, End: nil})
			found = true
			break
		}
	}
	if !found {
		states = append(states, state.TimeBoxState{
			TaskHash: taskHash,
			Segments: []state.TimeSegment{{Start: startTime, End: nil}},
		})
	}

	// Write the updated state list back to disk
	writeState := func(states []state.TimeBoxState) {
		if f, err := os.Create(stateFile); err == nil {
			defer f.Close()
			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			if err := enc.Encode(states); err != nil {
				fmt.Printf("Warning: Could not write state file: %v\n", err)
			}
		} else {
			fmt.Printf("Warning: Could not create state file: %v\n", err)
		}
	}
	writeState(states)
	// --- END STATE MANAGEMENT ---

	// --- SIGNAL HANDLING: Pause timebox and save state on SIGINT/SIGTERM ---
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)


	go func() {
		sig := <-sigChan
		fmt.Printf("\nReceived signal: %v. Pausing timebox and saving state...\n", sig)
		now := time.Now()
		// Find the current task's state and close the last segment if it's still open
		for i := range states {
			if states[i].TaskHash == taskHash && len(states[i].Segments) > 0 {
				lastSeg := &states[i].Segments[len(states[i].Segments)-1]
				if lastSeg.End == nil {
					lastSeg.End = &now
				}
			}
		}
		writeState(states)

		os.Exit(130) // 128 + SIGINT
	}()
	// --- END SIGNAL HANDLING ---

	stopChan := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	go timerAndGitWatcher(nextTask.Description, actualDuration, actualEndTime, startTime, stopChan, &wg)

	// Wait for user input to finish early or timer to expire
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadBytes('\n') // This will block until Enter is pressed

	// Signal the goroutine to stop
	select {
	case stopChan <- struct{}{}:
	default: // Non-blocking send in case the goroutine already exited (e.g., timer expired)
	}
	wg.Wait() // Wait for the goroutine to finish its cleanup

	finalEndTime := time.Now() // Record the actual end time of the task

	// Fetch all commits made during the task's active period
	allCommits, err := gitutil.GetCommitsSince(startTime) // Use gitutil package
	if err != nil {
		fmt.Printf("Warning: Could not fetch Git commits: %v\n", err)
		// Continue even if git commits fail, just don't add them
		allCommits = []string{}
	}

	// Filter commits to only include those made *during* the task's actual run time
	var commitsDuringTask []string
	for _, commitLine := range allCommits {
		commitsDuringTask = append(commitsDuringTask, commitLine)
	}

	nextTask.IsChecked = true // Mark the task as completed

	// Update the markdown file, passing startTime and finalEndTime
	err = parser.UpdateMarkdown(markdownFile, *nextTask, commitsDuringTask, startTime, finalEndTime) // Use parser package

	if err != nil {
		fmt.Printf("Error updating markdown file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nTask completed and markdown updated!")
}
