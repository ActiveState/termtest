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
		closed:  make(chan struct{}, 1),
	}

	for _, optSetter := range opts {
		optSetter(oc.opts)
	}

	return oc
}

// Report will consume the given buffer and will block unless Wait() has been called
func (e *outputConsumer) Report(buffer []byte) (stopConsuming bool, err error) {
	if e.isClosed() {
		return false, StoppedError
	}

	stop, err := e.consume(string(buffer))
	if err != nil {
		err = fmt.Errorf("meets threw error: %w", err)
	}
	if err != nil || stop {
		e.opts.Logger.Printf("Closing consumer: stop: %v, err: %v", stop, err)
		go func() {
			e.opts.Logger.Printf("Sending err to waiter")
			// This prevents report() from blocking in case Wait() has not been called yet
			e.waiter <- err
		}()
	}
	return stop, err
}

func (e *outputConsumer) Close() {
	if e.isClosed() {
		return
	}
	e.opts.Logger.Println("Closing output listener")
	defer e.opts.Logger.Println("Closed output listener")

	e.opts.Logger.Printf("Premature error")
	e.waiter <- StopPrematureError

	e.opts.Logger.Printf("Closing channel from Close()")
	close(e.closed)
}

func (e *outputConsumer) isClosed() (r bool) {
	defer e.opts.Logger.Printf("isClosed: %v", r)
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
			e.opts.Logger.Println("Closing channel from Wait()")
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
