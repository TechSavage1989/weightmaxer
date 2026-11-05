package update

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Policy holds the check timing.
type Policy struct {
	Interval       time.Duration // automatic checks while the window is open and not minimized
	FocusMin       time.Duration // returning to the window checks if this long has passed
	ManualCooldown time.Duration // minimum gap between "Check now" presses
	BackoffMax     time.Duration // longest wait after repeated failures
	Tick           time.Duration // how often the timer re-evaluates
}

// DefaultPolicy: at startup, every five minutes, and on focus after two minutes.
var DefaultPolicy = Policy{
	Interval:       5 * time.Minute,
	FocusMin:       2 * time.Minute,
	ManualCooldown: 60 * time.Second,
	BackoffMax:     60 * time.Minute,
	Tick:           15 * time.Second,
}

// Trigger says why a check is being considered.
type Trigger int

const (
	TriggerStartup Trigger = iota
	TriggerTimer
	TriggerFocus
	TriggerManual
)

// SchedState is what the scheduler remembers between checks.
type SchedState struct {
	LastAttempt time.Time
	LastManual  time.Time
	Failures    int
	PauseUntil  time.Time
	InFlight    bool
}

// wait is the automatic interval after n consecutive failures.
func (p Policy) wait(failures int) time.Duration {
	d := p.Interval
	for i := 0; i < failures && d < p.BackoffMax; i++ {
		d *= 2
	}
	if d > p.BackoffMax {
		d = p.BackoffMax
	}
	return d
}

// Decide reports whether a check may start now. reason explains a refusal.
func (p Policy) Decide(t Trigger, now time.Time, s SchedState, active bool) (bool, string) {
	if s.InFlight {
		return false, "Already checking."
	}
	if now.Before(s.PauseUntil) {
		return false, fmt.Sprintf("GitHub asked Weightmaxer to wait. Next check after %s.", s.PauseUntil.Local().Format("3:04 PM"))
	}
	switch t {
	case TriggerStartup:
		return true, ""
	case TriggerTimer:
		return active && !s.LastAttempt.IsZero() && now.Sub(s.LastAttempt) >= p.wait(s.Failures), ""
	case TriggerFocus:
		return !s.LastAttempt.IsZero() && now.Sub(s.LastAttempt) >= p.FocusMin, ""
	case TriggerManual:
		if left := p.ManualCooldown - now.Sub(s.LastManual); !s.LastManual.IsZero() && left > 0 {
			return false, fmt.Sprintf("Checked just now. Try again in %d seconds.", int(left.Seconds()+0.999))
		}
		return true, ""
	}
	return false, ""
}

// Record updates the state with a finished check.
func (s *SchedState) Record(r Result) {
	s.InFlight = false
	if r.Kind.Failed() {
		s.Failures++
	} else {
		s.Failures = 0
	}
	if r.PauseUntil.After(s.PauseUntil) {
		s.PauseUntil = r.PauseUntil
	}
}

// Scheduler runs checks on real timers. Only one check runs at a time.
type Scheduler struct {
	Policy   Policy
	Now      func() time.Time
	Check    func(context.Context) Result
	Active   func() bool // window open and not minimized
	OnStart  func()
	OnResult func(r Result, manual bool)

	mu     sync.Mutex
	st     SchedState
	focus  chan struct{}
	manual chan struct{}
	once   sync.Once
}

func (s *Scheduler) init() {
	s.once.Do(func() {
		s.focus = make(chan struct{}, 1)
		s.manual = make(chan struct{}, 1)
		if s.Now == nil {
			// Wall-clock time without the monotonic reading, so time spent asleep counts.
			s.Now = func() time.Time { return time.Now().Round(0) }
		}
	})
}

// State returns a copy of the scheduler state.
func (s *Scheduler) State() SchedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st
}

// Focus tells the scheduler the window was brought back to the front.
func (s *Scheduler) Focus() {
	s.init()
	select {
	case s.focus <- struct{}{}:
	default:
	}
}

// Manual handles "Check now". It answers at once so the button can explain a refusal.
func (s *Scheduler) Manual() (bool, string) {
	s.init()
	s.mu.Lock()
	if s.st.InFlight {
		s.mu.Unlock()
		return true, "Checking…"
	}
	ok, why := s.Policy.Decide(TriggerManual, s.Now(), s.st, true)
	s.mu.Unlock()
	if ok {
		select {
		case s.manual <- struct{}{}:
		default:
		}
	}
	return ok, why
}

// Run blocks until ctx ends. The first check starts right away, without
// waiting for anything else the app does at startup.
func (s *Scheduler) Run(ctx context.Context) {
	s.init()
	s.try(ctx, TriggerStartup)
	tick := time.NewTicker(s.Policy.Tick)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.try(ctx, TriggerTimer)
		case <-s.focus:
			s.try(ctx, TriggerFocus)
		case <-s.manual:
			s.try(ctx, TriggerManual)
		}
	}
}

func (s *Scheduler) try(ctx context.Context, t Trigger) {
	s.mu.Lock()
	now := s.Now()
	active := t != TriggerTimer || s.Active == nil || s.Active()
	ok, _ := s.Policy.Decide(t, now, s.st, active)
	if !ok {
		s.mu.Unlock()
		return
	}
	s.st.InFlight = true
	s.st.LastAttempt = now
	if t == TriggerManual {
		s.st.LastManual = now
	}
	s.mu.Unlock()
	if s.OnStart != nil {
		s.OnStart()
	}
	// The check runs in its own goroutine so focus and manual requests keep
	// being received; Decide refuses them while InFlight is set.
	go func() {
		cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		r := s.Check(cctx)
		cancel()
		s.mu.Lock()
		s.st.Record(r)
		s.mu.Unlock()
		if s.OnResult != nil {
			s.OnResult(r, t == TriggerManual)
		}
	}()
}
