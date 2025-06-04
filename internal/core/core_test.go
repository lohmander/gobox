package core

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Helper to create a temporary markdown file with given content
func createTempMarkdownFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "tasks.md")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp markdown file: %v", err)
	}
	return tmpFile
}

// Helper to read file content as string
func readFileContent(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	return string(data)
}

// Helper to remove .gobox_state.json if it exists
func cleanupStateFile(t *testing.T, dir string) {
	t.Helper()
	stateFile := filepath.Join(dir, ".gobox_state.json")
	_ = os.Remove(stateFile)
}

// Helper to capture stdout/stderr during test
func captureOutput(f func()) (string, string) {
	origStdout := os.Stdout
	origStderr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	outC := make(chan string)
	errC := make(chan string)
	go func() {
		var sb strings.Builder
		io.Copy(&sb, rOut)
		outC <- sb.String()
	}()
	go func() {
		var sb strings.Builder
		io.Copy(&sb, rErr)
		errC <- sb.String()
	}()

	f()
	wOut.Close()
	wErr.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr
	outStr := <-outC
	errStr := <-errC
	return outStr, errStr
}

func TestStartGoBox_BasicFlow(t *testing.T) {
	content := "- [ ] Test Task @1m\n"
	tmpFile := createTempMarkdownFile(t, content)

	// Use in-memory state store for testability (no disk I/O)
	memStore := NewInMemoryStateStore()

	// Simulate user pressing Enter immediately by running in a goroutine and sending newline to stdin
	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r

	done := make(chan struct{})
	go func() {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte("\n"))
		w.Close()
		close(done)
	}()

	out, err := captureOutput(func() {
		StartGoBoxWithClockAndStore(tmpFile, nil, memStore)
	})

	os.Stdin = origStdin
	<-done

	updated := readFileContent(t, tmpFile)
	if !strings.Contains(updated, "[x] Test Task @1m") {
		t.Errorf("Task was not checked as completed: %q", updated)
	}
	if !strings.Contains(updated, "⏱️") {
		t.Errorf("Duration not recorded in markdown: %q", updated)
	}
	if !strings.Contains(out, "Task completed and markdown updated!") {
		t.Errorf("Expected completion message in output: %q", out)
	}
	if err != "" {
		t.Errorf("Expected no stderr output, got: %q", err)
	}
}

func TestStartGoBox_StatePopulatedDuringActiveSession(t *testing.T) {
	content := "- [ ] Test Task @1m\n"
	tmpFile := createTempMarkdownFile(t, content)
	memStore := NewInMemoryStateStore()

	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r

	done := make(chan struct{})
	go func() {
		// Wait a bit before sending Enter to simulate an active session
		time.Sleep(300 * time.Millisecond)
		w.Write([]byte("\n"))
		w.Close()
		close(done)
	}()

	// Start GoBox in a goroutine so we can check state while it's running
	go func() {
		StartGoBoxWithClockAndStore(tmpFile, nil, memStore)
	}()

	// Wait a short moment to let the session start
	time.Sleep(100 * time.Millisecond)

	// Check that the state is populated during the session
	states, stateErr := memStore.Load()
	if stateErr != nil {
		t.Errorf("Expected state to be loadable, but got error: %v", stateErr)
	} else if len(states) == 0 {
		t.Errorf("Expected in-memory state to be non-empty during active session")
	}

	<-done
	os.Stdin = origStdin
}

func TestStartGoBox_NoTasks(t *testing.T) {
	content := "- [x] Done Task @1h\n"
	tmpFile := createTempMarkdownFile(t, content)
	tmpDir := filepath.Dir(tmpFile)
	cleanupStateFile(t, tmpDir)

	out, err := captureOutput(func() {
		StartGoBox(tmpFile)
	})

	if !strings.Contains(out, "No unchecked tasks with time boxes found") {
		t.Errorf("Expected message for no tasks, got: %q", out)
	}
	if err != "" {
		t.Errorf("Expected no stderr output, got: %q", err)
	}
}

// Additional tests for pause/resume, state file, and error cases can be added here.
