package termtest

import (
	"fmt"
	"time"
)

var StopPrematureError = fmt.Errorf("stop called while consumer was still active")
var StoppedError = fmt.Errorf("consumer has stopped")

type consumer func(buffer string) (keepConsuming bool, err error)

type outputConsumer struct {
	_test_id string // for testing purposes only, not used for non-testing logic
	timeout  time.Duration
	consume  consumer
	closed   chan struct{}
	waiter   chan error
	pos      int
	opts     *Opts
}

func newOutputConsumer(consume consumer, timeout time.Duration, opts *Opts) *outputConsumer {
	return &outputConsumer{
		timeout: timeout,
		consume: consume,
		opts:    opts,
		waiter:  make(chan error, 1),
		closed:  make(chan struct{}),
	}
}

// report will consume the given buffer and will block unless Wait() has been called
func (e *outputConsumer) report(buffer []byte) (keepConsuming bool, err error) {
	if e.isClosed() {
		return false, StoppedError
	}

	keep, err := e.consume(string(buffer))
	if err != nil {
		err = fmt.Errorf("meets threw error: %w", err)
	}
	if err != nil || !keep {
		go func() {
			// This prevents report() from blocking in case Wait() has not been called yet
			e.waiter <- err
		}()
	}
	return keep, err
}

func (e *outputConsumer) close() {
	if e.isClosed() {
		return
	}
	e.opts.Logger.Println("Closing output listener")
	defer e.opts.Logger.Println("Closed output listener")

	e.waiter <- StopPrematureError
	close(e.closed)
}

func (e *outputConsumer) isClosed() bool {
	select {
	case <-e.closed:
		return true
	default:
		return false
	}
}

func (e *outputConsumer) Wait() error {
	e.opts.Logger.Println("Output listener started waiting")
	defer e.opts.Logger.Println("Output listener stopped waiting")
	defer func() {
		if !e.isClosed() {
			close(e.closed)
		}
	}()

	select {
	case err := <-e.waiter:
		return err
	case <-time.After(e.timeout):
		return fmt.Errorf("after %s: %w", e.timeout, TimeoutError)
	}
}
