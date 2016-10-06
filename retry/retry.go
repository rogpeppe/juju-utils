// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package retry provides a framework for retrying actions.
// It does not itself invoke the action to be retried, but
// is intended to be used in a retry loop.
//
// The basic usage is as follows:
//
//	for a := someStrategy.Start(); a.Next(); {
//		try()
//	}
//
// See examples for details of suggested usage.
package retry

import (
	"time"

	"github.com/juju/utils/clock"
)

// Strategy is implemented by types that represent a retry strategy.
type Strategy interface {
	NewTimer(now time.Time) Timer
}

// Timer represents a source of timing events for a retry strategy.
type Timer interface {
	// NextSleep returns the length of time to sleep
	// before the next retry. If no more attempts should
	// be made it should return false, and the returned
	// duration will be ignored.
	//
	// Note that NextSleep is called once after
	// each iteration has completed, assuming the
	// retry loop is continuing.
	NextSleep(now time.Time) (time.Duration, bool)
}

// Attempt represents a running retry attempt.
type Attempt struct {
	clock   clock.Clock
	stop    <-chan struct{}
	timer   Timer
	count   int
	waited  bool
	running bool
}

// Start begins a new sequence of attempts for the given strategy. If
// clk is nil, clock.WallClock will be used. If a value is received on
// stop while waiting, the attempt will be aborted.
func Start(strategy Strategy, clk clock.Clock, stop <-chan struct{}) *Attempt {
	if clk == nil {
		clk = clock.WallClock
	}
	return &Attempt{
		clock:   clk,
		stop:    stop,
		timer:   strategy.NewTimer(clk.Now()),
		waited:  true,
		running: true,
	}
}

// Next waits until it is time to perform the next attempt or returns
// false if it is time to stop trying.
// It always returns true the first time it is called - we are guaranteed to
// make at least one attempt.
func (a *Attempt) Next() bool {
	a.HasNext()
	a.waited = false
	if a.running {
		a.count++
	}
	return a.running
}

// Count returns the current attempt count number, starting at 1.
// It returns 0 if called before Next is called.
func (a *Attempt) Count() int {
	return a.count
}

// HasNext waits until it is time to perform the next attempt
// and returns the value that Next will return.
// Multiple consecutive calls to HasNext without
// an intervening Next call will not cause any further
// delays.
func (a *Attempt) HasNext() bool {
	if a.waited || !a.running {
		return a.running
	}
	a.waited = true
	sleep, ok := a.timer.NextSleep(a.clock.Now())
	if !ok {
		a.running = false
		return false
	}
	a.waited = true
	select {
	case <-a.clock.After(sleep):
	case <-a.stop:
		a.running = false
	}
	return a.running
}
