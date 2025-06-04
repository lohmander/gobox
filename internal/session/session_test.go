package session

import (
	"testing"
	"time"

	"gobox/internal/state"
	"gobox/pkg/task"
)

func TestSessionRunner_BasicFlow(t *testing.T) {
	// Setup
	tbTask := task.Task{
		Description: "Test Task",
		TimeBox:     "@2s",
		IsChecked:   false,
	}
	tbState := &state.TimeBoxState{
		TaskHash: tbTask.Hash(),
		Segments: []state.TimeSegment{},
	}
	duration := 2 * time.Second

	runner := NewSessionRunner(tbTask, tbState, duration, time.Time{})

	// Start session
	runner.Start()

	// Wait for completion event
	completed := false
	timeout := time.After(5 * time.Second)
	for !completed {
		select {
		case ev := <-runner.Events():
			if ev == EventCompleted {
				completed = true
			}
		case <-timeout:
			t.Fatal("Session did not complete in expected time")
		}
	}

	runner.Wait()

	// Check state: should have one segment, with End set
	if len(tbState.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(tbState.Segments))
	}
	seg := tbState.Segments[0]
	if seg.End == nil {
		t.Errorf("segment End should not be nil after completion")
	}
	elapsed := seg.End.Sub(seg.Start)
	if elapsed < duration || elapsed > duration+500*time.Millisecond {
		t.Errorf("unexpected elapsed duration: got %v, want ~%v", elapsed, duration)
	}
}

func TestSessionRunner_PauseResume(t *testing.T) {
	tbTask := task.Task{
		Description: "PauseResume Task",
		TimeBox:     "@3s",
		IsChecked:   false,
	}
	tbState := &state.TimeBoxState{
		TaskHash: tbTask.Hash(),
		Segments: []state.TimeSegment{},
	}
	duration := 3 * time.Second

	runner := NewSessionRunner(tbTask, tbState, duration, time.Time{})
	runner.Start()

	// Wait for 1 tick, then pause
	gotTick := false
	timeout := time.After(2 * time.Second)
	for !gotTick {
		select {
		case ev := <-runner.Events():
			if ev == EventTick {
				gotTick = true
			}
		case <-timeout:
			t.Fatal("Did not receive tick event in time")
		}
	}
	runner.Pause()

	// Wait a moment to ensure no more ticks
	select {
	case ev := <-runner.Events():
		if ev == EventTick {
			t.Error("Received tick after pause")
		}
	case <-time.After(1100 * time.Millisecond):
		// ok, no tick
	}

	// Resume and wait for completion
	runner.Resume()
	completed := false
	timeout = time.After(5 * time.Second)
	for !completed {
		select {
		case ev := <-runner.Events():
			if ev == EventCompleted {
				completed = true
			}
		case <-timeout:
			t.Fatal("Session did not complete after resume")
		}
	}
	runner.Wait()

	// Should have two segments (one before pause, one after)
	if len(tbState.Segments) != 2 {
		t.Errorf("expected 2 segments, got %d", len(tbState.Segments))
	}
	for i, seg := range tbState.Segments {
		if seg.End == nil {
			t.Errorf("segment %d End should not be nil after completion", i)
		}
	}
}

func TestSessionRunner_Stop(t *testing.T) {
	tbTask := task.Task{
		Description: "Stop Task",
		TimeBox:     "@10s",
		IsChecked:   false,
	}
	tbState := &state.TimeBoxState{
		TaskHash: tbTask.Hash(),
		Segments: []state.TimeSegment{},
	}
	duration := 10 * time.Second

	runner := NewSessionRunner(tbTask, tbState, duration, time.Time{})
	runner.Start()

	// Wait for a tick, then stop
	gotTick := false
	timeout := time.After(2 * time.Second)
	for !gotTick {
		select {
		case ev := <-runner.Events():
			if ev == EventTick {
				gotTick = true
			}
		case <-timeout:
			t.Fatal("Did not receive tick event in time")
		}
	}
	runner.Stop()

	// Should emit EventStopped
	stopped := false
	timeout = time.After(2 * time.Second)
	for !stopped {
		select {
		case ev := <-runner.Events():
			if ev == EventStopped {
				stopped = true
			}
		case <-timeout:
			t.Fatal("Did not receive EventStopped after Stop()")
		}
	}
	runner.Wait()
}

func TestSessionRunner_EndTime(t *testing.T) {
	tbTask := task.Task{
		Description: "EndTime Task",
		TimeBox:     "@[00:00-00:00]", // We'll fudge the end time to now+2s
		IsChecked:   false,
	}
	tbState := &state.TimeBoxState{
		TaskHash: tbTask.Hash(),
		Segments: []state.TimeSegment{},
	}
	endTime := time.Now().Add(2 * time.Second)

	runner := NewSessionRunner(tbTask, tbState, 0, endTime)
	runner.Start()

	completed := false
	timeout := time.After(5 * time.Second)
	for !completed {
		select {
		case ev := <-runner.Events():
			if ev == EventCompleted {
				completed = true
			}
		case <-timeout:
			t.Fatal("Session did not complete by end time")
		}
	}
	runner.Wait()
}
