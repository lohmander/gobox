package clock

import (
	"time"
)

// Clock interface for testable time control
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
	NewTicker(d time.Duration) Ticker
}

type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// RealClock implements Clock using the time package
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (RealClock) NewTicker(d time.Duration) Ticker {
	t := time.NewTicker(d)
	return &realTicker{t}
}

type realTicker struct{ *time.Ticker }

func (t *realTicker) C() <-chan time.Time { return t.Ticker.C }
func (t *realTicker) Stop()               { t.Ticker.Stop() }
