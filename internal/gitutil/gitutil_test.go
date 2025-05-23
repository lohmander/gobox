package gitutil_test

import (
	"context"
	"testing"
	"time"

	"gobox/internal/gitutil"
)

// MockRunner for testing command execution.
type MockRunner struct {
	output string
	err    error
}

func (m MockRunner) CombinedOutput(ctx context.Context, name string, arg ...string) ([]byte, error) {
	return []byte(m.output), m.err
}

func TestGetCommitsSince(t *testing.T) {
	now := time.Now()
	since := now.Add(-time.Hour)
	sinceStr := since.Format(time.RFC3339)

	tests := []struct {
		name        string
		mockOutput  string
		mockError   error
		wantCommits []string
		wantErr     bool
		checkArgs   []string
	}{
		{
			name:        "successful with commits",
			mockOutput:  "abcdefg Commit 1\nhijklmn Another commit",
			wantCommits: []string{"abcdefg Commit 1", "hijklmn Another commit"},
			wantErr:     false,
			checkArgs:   []string{"log", "--oneline", "--since", sinceStr},
		},
		{
			name:        "no commits",
			mockOutput:  "",
			wantCommits: []string{},
			wantErr:     false,
			checkArgs:   []string{"log", "--oneline", "--since", sinceStr},
		},
		{
			name:        "not a git repository",
			mockOutput:  "fatal: not a git repository (or any of the parent directories): .git",
			wantErr:     true,
			wantCommits: nil,
			checkArgs:   []string{"log", "--oneline", "--since", sinceStr},
		},
		{
			name:        "other git error",
			mockOutput:  "error: something went wrong",
			mockError:   &mockExecError{output: "error: something went wrong"},
			wantErr:     true,
			wantCommits: nil,
			checkArgs:   []string{"log", "--oneline", "--since", sinceStr},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := MockRunner{output: tt.mockOutput, err: tt.mockError}
			gitutil.SetRunner(mockRunner)
			defer gitutil.SetRunner(gitutil.DefaultRunner{}) // Reset after test

			gotCommits, err := gitutil.GetCommitsSince(since)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCommitsSince() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(gotCommits) != len(tt.wantCommits) {
				t.Errorf("GetCommitsSince() gotCommits length = %d, wantCommits length = %d", len(gotCommits), len(tt.wantCommits))
				return
			}
		})
	}
}

// Mock error to simulate exec command errors
type mockExecError struct {
	output string
}

func (e *mockExecError) Error() string {
	return "mock exec error: " + e.output
}

func (e *mockExecError) Unwrap() error {
	return nil
}
