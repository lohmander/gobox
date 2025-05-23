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

// For testing, we'll add a function to set a mock runner
func SetRunner(r CommandRunner) {
	runner = r
}
