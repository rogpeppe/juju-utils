package retry_test

import (
	"net/http"
	"time"

	"github.com/juju/utils/retry"
)

func doSomething() (int, error) { return 0, nil }

func shouldRetry(error) bool { return false }

func doSomethingWith(int) {}

func ExampleAttempt_HasNext() {
	// This example shows how Attempt.HasNext can be used to help
	// structure an attempt loop. If the godoc example code allowed
	// us to make the example return an error, we would uncomment
	// the commented return statements.
	attempts := retry.Regular{
		Total: 1 * time.Second,
		Delay: 250 * time.Millisecond,
	}
	for attempt := attempts.Start(nil, nil); attempt.Next(); {
		x, err := doSomething()
		if shouldRetry(err) && attempt.HasNext() {
			continue
		}
		if err != nil {
			// return err
			return
		}
		doSomethingWith(x)
	}
	// return ErrTimedOut
	return
}

func ExampleExponential() {
	// This example shows a retry loop that will retry an
	// HTTP POST request with an exponential backoff
	// for up to 30s.
	strategy := retry.LimitTime(30*time.Second,
		retry.Exponential{
			Initial: 10 * time.Millisecond,
			Factor:  1.5,
		},
	)
	for a := retry.Start(strategy, nil, nil); a.Next(); {
		if reply, err := http.Post("http://example.com/form", "", nil); err == nil {
			reply.Body.Close()
			break
		}
	}
}

//
// For example, to retry an HTTP POST request every half second
// for up to 10s:
//
//	strategy := retry.
