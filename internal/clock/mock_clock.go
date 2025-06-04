package clock

import (
	"sync"
	"time"
)

// MockClock allows manual control of time for testing.
type MockClock struct {
	mu      sync.Mutex
	now     time.Time
	tickers []*MockTicker
}

// NewMockClock creates a MockClock starting at the given time.
func NewMockClock(start time.Time) *MockClock {
	return &MockClock{
		now: start,
	}
}

func (c *MockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *MockClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	go func() {
		c.mu.Lock()
		target := c.now.Add(d)
		c.mu.Unlock()
		// In tests, you should call Advance to reach this time.
		for {
			c.mu.Lock()
			if !c.now.Before(target) {
				ch <- c.now
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()
			time.Sleep(1 * time.Millisecond)
		}
	}()
	return ch
}

func (c *MockClock) NewTicker(d time.Duration) Ticker {
	t := &MockTicker{
		C_:      make(chan time.Time, 100),
		clock:   c,
		period:  d,
		stopped: false,
	}
	c.mu.Lock()
	c.tickers = append(c.tickers, t)
	c.mu.Unlock()
	return t
}

// Advance moves the clock forward by d, firing any tickers as needed.
func (c *MockClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	tickers := append([]*MockTicker(nil), c.tickers...)
	c.mu.Unlock()
	for _, t := range tickers {
		t.tickIfDue()
	}
}

// MockTicker implements Ticker for MockClock.
type MockTicker struct {
	C_      chan time.Time
	clock   *MockClock
	period  time.Duration
	last    time.Time
	stopped bool
	mu      sync.Mutex
}

func (t *MockTicker) C() <-chan time.Time {
	return t.C_
}

func (t *MockTicker) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = true
	close(t.C_)
}

func (t *MockTicker) tickIfDue() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	t.clock.mu.Lock()
	now := t.clock.now
	t.clock.mu.Unlock()
	if t.last.IsZero() {
		t.last = now
	}
	for !t.last.Add(t.period).After(now) {
		t.last = t.last.Add(t.period)
		select {
		case t.C_ <- t.last:
		default:
		}
	}
}
