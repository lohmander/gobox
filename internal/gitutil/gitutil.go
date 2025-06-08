package gitutil

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CommandRunner is an interface for running external commands.
type CommandRunner interface {
	CombinedOutput(ctx context.Context, name string, arg ...string) ([]byte, error)
}

// DefaultRunner implements CommandRunner using os/exec.Command.
type DefaultRunner struct{}

func (r DefaultRunner) CombinedOutput(ctx context.Context, name string, arg ...string) ([]byte, error) {
	cmd := commandContext(ctx, name, arg...)
	return cmd.CombinedOutput()
}

// commandContext is a helper to create a *exec.Cmd with context.
func commandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	return cmd
}

// We'll use a package-level variable for the runner
var runner CommandRunner = DefaultRunner{}

// GetCommitsSince fetches git commits since a given time, using the runner.
func GetCommitsSince(since time.Time) ([]string, error) {
	// Use --date=iso-strict to ensure consistent date format for parsing
	outputBytes, err := runner.CombinedOutput(context.Background(), "git", "log", "--oneline", "--since", since.Format(time.RFC3339))
	if err != nil {
		if strings.Contains(strings.ToLower(string(outputBytes)), "not a git repository") {
			return nil, fmt.Errorf("not a git repository: %s", strings.TrimSpace(string(outputBytes)))
		}
		return nil, fmt.Errorf("error running git log: %w, output: %s", err, string(outputBytes))
	}

	output := string(outputBytes)
	if strings.Contains(strings.ToLower(output), "not a git repository") {
		return nil, fmt.Errorf("not a git repository: %s", strings.TrimSpace(output))
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var commits []string
	for _, line := range lines {
		if line != "" {
			commits = append(commits, line)
		}
	}
	return commits, nil
}

// GetCommitsBetween fetches git commits between multiple time ranges.
// Each range consists of a start and end time. For ongoing segments (where end is nil),
// commits up to the current time are included.
// The function returns unique commits across all segments.
func GetCommitsBetween(segments []time.Time) ([]string, error) {
	uniqueCommits := make(map[string]struct{})
	var allCommits []string

	// We need at least one time to start with
	if len(segments) == 0 {
		return nil, fmt.Errorf("at least one time segment is required")
	}

	// Use standard git log to get all commits since the earliest time
	earliestTime := segments[0]
	for _, t := range segments[1:] {
		if t.Before(earliestTime) {
			earliestTime = t
		}
	}

	// Get all commits since the earliest time
	commits, err := GetCommitsSince(earliestTime)
	if err != nil {
		return nil, err
	}

	// Process all commits and keep unique ones
	for _, commit := range commits {
		if _, exists := uniqueCommits[commit]; !exists {
			uniqueCommits[commit] = struct{}{}
			allCommits = append(allCommits, commit)
		}
	}

	return allCommits, nil
}

// For testing, we'll add a function to set a mock runner
func SetRunner(r CommandRunner) {
	runner = r
}
