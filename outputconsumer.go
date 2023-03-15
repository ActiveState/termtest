package termtest

import (
	"fmt"
	"time"
)

var StopPrematureError = fmt.Errorf("stop called while consumer was still active")
var StoppedError = fmt.Errorf("consumer has stopped")

type consumer func(buffer string) (stopConsuming bool, err error)

type outputConsumer struct {
	_test_id string // for testing purposes only, not used for non-testing logic
	timeout  time.Duration
	consume  consumer
	closed   chan struct{}
	waiter   chan error
	pos      int // Todo: Find a way to move the responsibility of this entirely into outputconsumer
	opts     *OutputConsumerOpts
}

type OutputConsumerOpts struct {
	*Opts

	// Sends the full buffer each time, with the latest data appended to the end.
	// This is the full buffer as of the point in time that the consumer started listening.
	SendFullBuffer bool
}

type SetConsOpt func(o *OutputConsumerOpts)

func OptInherit(o *Opts) func(o *OutputConsumerOpts) {
	return func(oco *OutputConsumerOpts) {
		oco.Opts = o
	}
}

func OptSendFullBuffer() func(o *OutputConsumerOpts) {
	return func(oco *OutputConsumerOpts) {
		oco.SendFullBuffer = true
	}
}

func newOutputConsumer(consume consumer, timeout time.Duration, opts ...SetConsOpt) *outputConsumer {
	oc := &outputConsumer{
		timeout: timeout,
		consume: consume,
		opts:    &OutputConsumerOpts{Opts: NewOpts()},
		waiter:  make(chan error, 1),
	}

	for _, optSetter := range opts {
		optSetter(oc.opts)
	}

	return oc
}

// Report will consume the given buffer and will block unless Wait() has been called
func (e *outputConsumer) Report(buffer []byte) (stopConsuming bool, err error) {
	stop, err := e.consume(string(buffer))
	if err != nil {
		err = fmt.Errorf("meets threw error: %w", err)
	}
	if err != nil || stop {
		go func() {
			// This prevents report() from blocking in case Wait() has not been called yet
			e.waiter <- err
		}()
	}
	return stop, err
}

func (e *outputConsumer) Close() {
	e.opts.Logger.Println("closing")
	if isClosed(e.waiter) {
		e.opts.Logger.Println("already closed")
		return
	}

	defer e.opts.Logger.Println("closed prematurely")

	e.waiter <- StopPrematureError
}

func (e *outputConsumer) Wait() error {
	e.opts.Logger.Println("started waiting")
	defer e.opts.Logger.Println("stopped waiting")

	select {
	case err := <-e.waiter:
		return err
	case <-time.After(e.timeout):
		return fmt.Errorf("after %s: %w", e.timeout, TimeoutError)
	}
}
