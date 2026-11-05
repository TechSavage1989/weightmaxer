package update

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	p := DefaultPolicy
	t0 := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	s := SchedState{LastAttempt: t0}
	if ok, _ := p.Decide(TriggerTimer, t0.Add(4*time.Minute), s, true); ok {
		t.Fatal("timer before five minutes")
	}
	if ok, _ := p.Decide(TriggerTimer, t0.Add(5*time.Minute), s, true); !ok {
		t.Fatal("timer at five minutes")
	}
	if ok, _ := p.Decide(TriggerTimer, t0.Add(5*time.Minute), s, false); ok {
		t.Fatal("timer while minimized")
	}
	if ok, _ := p.Decide(TriggerFocus, t0.Add(119*time.Second), s, true); ok {
		t.Fatal("focus before two minutes")
	}
	if ok, _ := p.Decide(TriggerFocus, t0.Add(2*time.Minute), s, true); !ok {
		t.Fatal("focus after two minutes")
	}
	// Backoff after failures: 10, 20, 40, then capped at 60 minutes.
	for fails, wait := range map[int]time.Duration{1: 10 * time.Minute, 2: 20 * time.Minute, 3: 40 * time.Minute, 6: 60 * time.Minute} {
		s := SchedState{LastAttempt: t0, Failures: fails}
		if ok, _ := p.Decide(TriggerTimer, t0.Add(wait-time.Second), s, true); ok {
			t.Fatalf("failures=%d checked early", fails)
		}
		if ok, _ := p.Decide(TriggerTimer, t0.Add(wait), s, true); !ok {
			t.Fatalf("failures=%d never checked", fails)
		}
	}
	// Pauses block every trigger, including manual.
	s = SchedState{LastAttempt: t0, PauseUntil: t0.Add(time.Hour)}
	for _, tr := range []Trigger{TriggerStartup, TriggerTimer, TriggerFocus, TriggerManual} {
		if ok, _ := p.Decide(tr, t0.Add(30*time.Minute), s, true); ok {
			t.Fatalf("trigger %d ran during pause", tr)
		}
	}
	// Manual cooldown.
	s = SchedState{LastManual: t0}
	ok, why := p.Decide(TriggerManual, t0.Add(20*time.Second), s, true)
	if ok || why != "Checked just now. Try again in 40 seconds." {
		t.Fatalf("cooldown = %v %q", ok, why)
	}
	if ok, _ := p.Decide(TriggerManual, t0.Add(time.Minute), s, true); !ok {
		t.Fatal("manual after cooldown")
	}
	if ok, _ := p.Decide(TriggerManual, t0, SchedState{InFlight: true}, true); ok {
		t.Fatal("ran while in flight")
	}
}

// TestSchedulerRealTimers runs the scheduler on real tickers and goroutines
// with shortened intervals: startup check, periodic checks, focus return,
// single flight and manual cooldown.
func TestSchedulerRealTimers(t *testing.T) {
	var checks int32
	var mu sync.Mutex
	var concurrent, maxConcurrent int
	release := make(chan struct{}, 100)
	s := &Scheduler{
		Policy: Policy{Interval: 300 * time.Millisecond, FocusMin: 150 * time.Millisecond, ManualCooldown: 400 * time.Millisecond, BackoffMax: time.Second, Tick: 20 * time.Millisecond},
		Active: func() bool { return true },
		Check: func(ctx context.Context) Result {
			mu.Lock()
			concurrent++
			if concurrent > maxConcurrent {
				maxConcurrent = concurrent
			}
			mu.Unlock()
			atomic.AddInt32(&checks, 1)
			select {
			case <-release:
			case <-time.After(30 * time.Millisecond):
			}
			mu.Lock()
			concurrent--
			mu.Unlock()
			return Result{Kind: KindUpToDate}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, func() bool { return atomic.LoadInt32(&checks) == 1 }, "startup check")
	// Focus right after startup is ignored (less than FocusMin).
	s.Focus()
	time.Sleep(80 * time.Millisecond)
	if n := atomic.LoadInt32(&checks); n != 1 {
		t.Fatalf("focus too soon triggered a check (%d)", n)
	}
	waitFor(t, func() bool { return atomic.LoadInt32(&checks) >= 2 }, "timer check")
	time.Sleep(200 * time.Millisecond)
	before := atomic.LoadInt32(&checks)
	s.Focus()
	waitFor(t, func() bool { return atomic.LoadInt32(&checks) > before }, "focus check")
	// Manual, then a second press inside the cooldown is refused with a reason.
	time.Sleep(60 * time.Millisecond)
	if ok, _ := s.Manual(); !ok {
		t.Fatal("manual refused")
	}
	waitFor(t, func() bool { return s.State().LastManual.After(time.Time{}) && !s.State().InFlight }, "manual check")
	if ok, why := s.Manual(); ok || why == "" {
		t.Fatalf("second manual = %v %q", ok, why)
	}
	mu.Lock()
	defer mu.Unlock()
	if maxConcurrent != 1 {
		t.Fatalf("checks overlapped: %d", maxConcurrent)
	}
}

func TestSchedulerSkipsWhileMinimized(t *testing.T) {
	var checks int32
	var active atomic.Bool
	s := &Scheduler{
		Policy: Policy{Interval: 50 * time.Millisecond, FocusMin: time.Hour, ManualCooldown: time.Hour, BackoffMax: time.Second, Tick: 10 * time.Millisecond},
		Active: func() bool { return active.Load() },
		Check:  func(context.Context) Result { atomic.AddInt32(&checks, 1); return Result{Kind: KindUpToDate} },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	time.Sleep(250 * time.Millisecond)
	if n := atomic.LoadInt32(&checks); n != 1 {
		t.Fatalf("minimized window checked %d times", n)
	}
	active.Store(true)
	waitFor(t, func() bool { return atomic.LoadInt32(&checks) >= 2 }, "check after restore")
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
