package gitutil

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GetCommitsSince fetches git commits since a given time.
func GetCommitsSince(since time.Time) ([]string, error) {
	// Use --date=iso-strict to ensure consistent date format for parsing
	cmd := exec.Command("git", "log", "--oneline", "--since", since.Format(time.RFC3339))
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If it's not a git repo, or no commits, this might error.
		// Return empty list rather than error if it's just no commits.
		if strings.Contains(strings.ToLower(string(output)), "not a git repository") {
			return nil, fmt.Errorf("not a git repository: %s", strings.TrimSpace(string(output)))
		}
		return nil, fmt.Errorf("error running git log: %w, output: %s", err, string(output))
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var commits []string
	for _, line := range lines {
		if line != "" {
			commits = append(commits, line)
		}
	}
	return commits, nil
}
