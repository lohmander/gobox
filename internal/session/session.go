package session

import (
	"sync"
	"time"

	"gobox/internal/state"
	"gobox/pkg/task"
)

// SessionEvent represents an event emitted by the session runner.
type SessionEvent int

const (
	EventTick SessionEvent = iota
	EventPaused
	EventResumed
	EventCompleted
	EventStopped
)

// SessionRunner manages a timeboxed session for a task, including pause/resume and segment tracking.
type SessionRunner struct {
	Task         task.Task
	State        *state.TimeBoxState
	Duration     time.Duration // total timebox duration (if duration-based)
	EndTime      time.Time     // absolute end time (if time-range-based)
	Ticker       *time.Ticker
	Mutex        sync.Mutex
	Paused       bool
	Completed    bool
	eventCh      chan SessionEvent
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// NewSessionRunner creates a new session runner for a task and its state.
func NewSessionRunner(task task.Task, tbState *state.TimeBoxState, duration time.Duration, endTime time.Time) *SessionRunner {
	return &SessionRunner{
		Task:     task,
		State:    tbState,
		Duration: duration,
		EndTime:  endTime,
		eventCh:  make(chan SessionEvent, 10),
		stopCh:   make(chan struct{}),
	}
}

// Start begins the session timer and emits tick events every second.
func (sr *SessionRunner) Start() {
	sr.Mutex.Lock()
	if sr.Paused || sr.Completed {
		sr.Mutex.Unlock()
		return
	}
	// Start a new segment if not already running
	if len(sr.State.Segments) == 0 || sr.State.Segments[len(sr.State.Segments)-1].End != nil {
		now := time.Now()
		sr.State.Segments = append(sr.State.Segments, state.TimeSegment{Start: now, End: nil})
	}
	sr.Ticker = time.NewTicker(1 * time.Second)
	sr.wg.Add(1)
	sr.Mutex.Unlock()

	go func() {
		defer sr.wg.Done()
		for {
			select {
			case <-sr.Ticker.C:
				sr.eventCh <- EventTick
				if sr.isTimeUp() {
					sr.Complete()
					return
				}
			case <-sr.stopCh:
				return
			}
		}
	}()
}

// Pause pauses the session and closes the current segment.
func (sr *SessionRunner) Pause() {
	sr.Mutex.Lock()
	defer sr.Mutex.Unlock()
	if sr.Paused || sr.Completed {
		return
	}
	now := time.Now()
	if len(sr.State.Segments) > 0 {
		last := &sr.State.Segments[len(sr.State.Segments)-1]
		if last.End == nil {
			last.End = &now
		}
	}
	sr.Paused = true
	if sr.Ticker != nil {
		sr.Ticker.Stop()
	}
	sr.eventCh <- EventPaused
}

// Resume resumes the session and starts a new segment.
func (sr *SessionRunner) Resume() {
	sr.Mutex.Lock()
	defer sr.Mutex.Unlock()
	if !sr.Paused || sr.Completed {
		return
	}
	now := time.Now()
	sr.State.Segments = append(sr.State.Segments, state.TimeSegment{Start: now, End: nil})
	sr.Paused = false
	sr.Ticker = time.NewTicker(1 * time.Second)
	sr.wg.Add(1)
	go func() {
		defer sr.wg.Done()
		for {
			select {
			case <-sr.Ticker.C:
				sr.eventCh <- EventTick
				if sr.isTimeUp() {
					sr.Complete()
					return
				}
			case <-sr.stopCh:
				return
			}
		}
	}()
	sr.eventCh <- EventResumed
}

// Complete ends the session, closes the current segment, and emits EventCompleted.
func (sr *SessionRunner) Complete() {
	sr.Mutex.Lock()
	defer sr.Mutex.Unlock()
	if sr.Completed {
		return
	}
	now := time.Now()
	if len(sr.State.Segments) > 0 {
		last := &sr.State.Segments[len(sr.State.Segments)-1]
		if last.End == nil {
			last.End = &now
		}
	}
	sr.Completed = true
	if sr.Ticker != nil {
		sr.Ticker.Stop()
	}
	
	// Prevent panics from double-closing the channel
	select {
	case _, ok := <-sr.stopCh:
		if !ok {
			// Channel already closed, don't close again or send event
			return
		}
		// Channel still open, close it
		close(sr.stopCh)
	default:
		// Channel still open, close it
		close(sr.stopCh)
	}
	
	// Only send event if channel is not full
	select {
	case sr.eventCh <- EventCompleted:
		// Successfully sent event
	default:
		// Cannot send, channel might be full or closed
	}
}

// Stop ends the session without marking it as completed.
func (sr *SessionRunner) Stop() {
	sr.Mutex.Lock()
	defer sr.Mutex.Unlock()
	
	// If already completed, don't do anything
	if sr.Completed {
		return
	}
	
	if sr.Ticker != nil {
		sr.Ticker.Stop()
	}
	
	// Prevent panics from double-closing the channel
	select {
	case _, ok := <-sr.stopCh:
		if !ok {
			// Channel already closed, don't close again
			return
		}
		// Channel still open, close it
		close(sr.stopCh)
	default:
		// Channel still open, close it
		close(sr.stopCh)
	}
	
	// Only send event if stopCh was closed by us
	select {
	case sr.eventCh <- EventStopped:
		// Successfully sent event
	default:
		// Cannot send, channel might be full or closed
	}
}

// Wait blocks until the session goroutine(s) have finished.
func (sr *SessionRunner) Wait() {
	sr.wg.Wait()
}

// Events returns the channel for receiving session events.
func (sr *SessionRunner) Events() <-chan SessionEvent {
	return sr.eventCh
}

// isTimeUp checks if the session has reached its duration or end time.
func (sr *SessionRunner) isTimeUp() bool {
	if sr.Duration > 0 {
		var elapsed time.Duration
		for _, seg := range sr.State.Segments {
			if seg.End != nil {
				elapsed += seg.End.Sub(seg.Start)
			} else {
				elapsed += time.Since(seg.Start)
			}
		}
		return elapsed >= sr.Duration
	} else if !sr.EndTime.IsZero() {
		return time.Now().After(sr.EndTime)
	}
	return false
}

// TotalElapsed returns the total elapsed time across all segments.
func (sr *SessionRunner) TotalElapsed() time.Duration {
	var elapsed time.Duration
	for _, seg := range sr.State.Segments {
		if seg.End != nil {
			elapsed += seg.End.Sub(seg.Start)
		} else {
			elapsed += time.Since(seg.Start)
		}
	}
	return elapsed
}

// Remaining returns the time remaining in the session.
// For duration-based sessions, it returns the duration minus elapsed time.
// For end-time-based sessions, it returns the time until the end time.
// Returns zero if the session is complete or if no time limit is set.
func (sr *SessionRunner) Remaining() time.Duration {
	sr.Mutex.Lock()
	defer sr.Mutex.Unlock()
	
	if sr.Completed {
		return 0
	}
	
	if sr.Duration > 0 {
		elapsed := sr.TotalElapsed()
		if elapsed >= sr.Duration {
			return 0
		}
		return sr.Duration - elapsed
	} else if !sr.EndTime.IsZero() {
		remaining := sr.EndTime.Sub(time.Now())
		if remaining < 0 {
			return 0
		}
		return remaining
	}
	return 0
}
