package clock

import (
	"testing"
	"time"
)

func TestRealClock_NowAndAfter(t *testing.T) {
	clk := RealClock{}
	before := time.Now()
	now := clk.Now()
	after := clk.After(10 * time.Millisecond)
	select {
	case <-after:
		// ok
	case <-time.After(100 * time.Millisecond):
		t.Error("RealClock.After did not fire within expected time")
	}
	if now.Before(before) || now.After(time.Now()) {
		t.Errorf("RealClock.Now returned unexpected time: %v", now)
	}
}

func TestRealClock_NewTicker(t *testing.T) {
	clk := RealClock{}
	ticker := clk.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	select {
	case <-ticker.C():
		// ok
	case <-time.After(100 * time.Millisecond):
		t.Error("RealClock.NewTicker.C did not fire within expected time")
	}
}

func TestRealTicker_Stop(t *testing.T) {
	clk := RealClock{}
	ticker := clk.NewTicker(10 * time.Millisecond)
	ticker.Stop()
	// After stopping, channel may or may not be closed, but should not panic
	select {
	case <-ticker.C():
	case <-time.After(20 * time.Millisecond):
		// ok, no panic
	}
}