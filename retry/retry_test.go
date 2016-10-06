// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package retry_test

import (
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/utils/clock"
	"github.com/juju/utils/retry"
)

type retrySuite struct{}

var _ = gc.Suite(&retrySuite{})

func (*retrySuite) TestAttemptTiming(c *gc.C) {
	testAttempt := retry.Regular{
		Total: 0.25e9,
		Delay: 0.1e9,
	}
	want := []time.Duration{0, 0.1e9, 0.2e9, 0.2e9}
	got := make([]time.Duration, 0, len(want)) // avoid allocation when testing timing
	t0 := time.Now()
	for a := testAttempt.Start(nil, nil); a.Next(); {
		got = append(got, time.Now().Sub(t0))
	}
	got = append(got, time.Now().Sub(t0))
	c.Assert(got, gc.HasLen, len(want))
	const margin = 0.01e9
	for i, got := range want {
		lo := want[i] - margin
		hi := want[i] + margin
		if got < lo || got > hi {
			c.Errorf("attempt %d want %g got %g", i, want[i].Seconds(), got.Seconds())
		}
	}
}

func (*retrySuite) TestAttemptNextHasNext(c *gc.C) {
	a := retry.Regular{}.Start(nil, nil)
	c.Assert(a.Next(), gc.Equals, true)
	c.Assert(a.Next(), gc.Equals, false)

	a = retry.Regular{}.Start(nil, nil)
	c.Assert(a.Next(), gc.Equals, true)
	c.Assert(a.HasNext(), gc.Equals, false)
	c.Assert(a.Next(), gc.Equals, false)

	a = retry.Regular{Total: 2e8}.Start(nil, nil)
	c.Assert(a.Next(), gc.Equals, true)
	c.Assert(a.HasNext(), gc.Equals, true)
	time.Sleep(2e8)
	c.Assert(a.HasNext(), gc.Equals, true)
	c.Assert(a.Next(), gc.Equals, true)
	c.Assert(a.Next(), gc.Equals, false)

	a = retry.Regular{Total: 1e8, Min: 2}.Start(nil, nil)
	time.Sleep(1e8)
	c.Assert(a.Next(), gc.Equals, true)
	c.Assert(a.HasNext(), gc.Equals, true)
	c.Assert(a.Next(), gc.Equals, true)
	c.Assert(a.HasNext(), gc.Equals, false)
	c.Assert(a.Next(), gc.Equals, false)
}

func (*retrySuite) TestAttemptWithStop(c *gc.C) {
	stop := make(chan struct{})
	close(stop)
	done := make(chan struct{})
	go func() {
		strategy := retry.Regular{
			Delay: 5 * time.Second,
			Total: 30 * time.Second,
		}
		n := 0
		for a := strategy.Start(nil, stop); a.Next(); {
			if n > 0 {
				c.Errorf("unexpected second attempt")
			}
			n++
		}
		c.Check(n, gc.Equals, 1)
		close(done)
	}()
	assertReceive(c, done, "attempt loop abort")
}

func (*retrySuite) TestAttemptWithMockClock(c *gc.C) {
	clock := testing.NewClock(time.Now())
	strategy := retry.Regular{
		Delay: 5 * time.Second,
		Total: 30 * time.Second,
	}
	progress := make(chan struct{})
	done := make(chan struct{})
	go func() {
		for a := strategy.Start(clock, nil); a.Next(); {
			progress <- struct{}{}
		}
		close(done)
	}()
	assertReceive(c, progress, "progress first time")
	clock.Advance(5 * time.Second)
	assertReceive(c, progress, "progress second time")
	clock.Advance(5 * time.Second)
	assertReceive(c, progress, "progress third time")
	clock.Advance(30 * time.Second)
	assertReceive(c, progress, "progress fourth time")
	assertReceive(c, done, "loop finish")
}

type strategyTest struct {
	about      string
	strategy   retry.Strategy
	calls      []nextCall
	terminates bool
}

type nextCall struct {
	// t holds the time since the timer was started that
	// the Next call will be made.
	t time.Duration
	// delay holds the length of time that a call made at
	// time t is expected to sleep for.
	sleep time.Duration
}

var strategyTests = []strategyTest{{
	about: "regular retry (same params as TestAttemptTiming)",
	strategy: retry.Regular{
		Total: 0.25e9,
		Delay: 0.1e9,
	},
	calls: []nextCall{
		{0, 0},
		{0, 0.1e9},
		{0.1e9, 0.1e9},
		{0.2e9, 0},
	},
	terminates: true,
}, {
	about: "regular retry with calls at different times",
	strategy: retry.Regular{
		Total: 2.5e9,
		Delay: 1e9,
	},
	calls: []nextCall{
		{0.5e9, 0},
		{0.5e9, 0.5e9},
		{1.1e9, 0.9e9},
		{2.2e9, 0},
	},
	terminates: true,
}, {
	about: "regular retry with call after next deadline",
	strategy: retry.Regular{
		Total: 3.5e9,
		Delay: 1e9,
	},
	calls: []nextCall{
		{0.5e9, 0},
		// We call Next at well beyond the deadline,
		// so we get a zero delay, but subsequent events
		// resume pace.
		{2e9, 0},
		{2.1e9, 0.9e9},
		{3e9, 0},
	},
	terminates: true,
}, {
	about: "exponential retry",
	strategy: retry.Exponential{
		Initial: 1e9,
		Factor:  2,
	},
	calls: []nextCall{
		{0, 0},
		{0.1e9, 0.9e9},
		{1e9, 2e9},
		{3e9, 4e9},
		{7e9, 8e9},
	},
}, {
	about: "time-limited exponential retry",
	strategy: retry.LimitTime(5e9, retry.Exponential{
		Initial: 1e9,
		Factor:  2,
	}),
	calls: []nextCall{
		{0, 0},
		{0.1e9, 0.9e9},
		{1e9, 2e9},
		{3e9, 0},
	},
	terminates: true,
}, {
	about: "count-limited exponential retry",
	strategy: retry.LimitCount(2, retry.Exponential{
		Initial: 1e9,
		Factor:  2,
	}),
	calls: []nextCall{
		{0, 0},
		{0.1e9, 0.9e9},
		{1e9, 0},
	},
	terminates: true,
}}

func (*retrySuite) TestStrategies(c *gc.C) {
	for i, test := range strategyTests {
		c.Logf("test %d: %s", i, test.about)
		testStrategy(c, test)
	}
}

func testStrategy(c *gc.C, test strategyTest) {
	t0 := time.Now()
	clk := &mockClock{
		now: t0,
	}
	a := retry.Start(test.strategy, clk, nil)
	for i, call := range test.calls {
		c.Logf("call %d - %v", i, call.t)
		clk.now = t0.Add(call.t)
		ok := a.Next()
		expectTerminate := test.terminates && i == len(test.calls)-1
		c.Assert(ok, gc.Equals, !expectTerminate)
		if got, want := clk.now.Sub(t0), call.t+call.sleep; !closeTo(got, want) {
			c.Fatalf("incorrect time after Next; got %v want %v", got, want)
		}
		if ok {
			c.Assert(a.Count(), gc.Equals, i+1)
		}
	}
}

// closeTo reports whether d0 and d1 are close enough
// to one another to cater for inaccuracies of floating point arithmetic.
func closeTo(d0, d1 time.Duration) bool {
	const margin = 20 * time.Nanosecond
	diff := d1 - d0
	if diff < 0 {
		diff = -diff
	}
	return diff < margin
}

type mockClock struct {
	clock.Clock

	now   time.Time
	sleep func(d time.Duration)
}

func (c *mockClock) After(d time.Duration) <-chan time.Time {
	c.now = c.now.Add(d)
	ch := make(chan time.Time)
	close(ch)
	return ch
}

func (c *mockClock) Now() time.Time {
	return c.now
}

func assertReceive(c *gc.C, ch <-chan struct{}, what string) {
	select {
	case <-ch:
	case <-time.After(time.Second):
		c.Fatalf("timed out waiting for %s", what)
	}
}
