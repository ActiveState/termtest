package termtest

import (
	"fmt"
	"time"
)

var StopPrematureError = fmt.Errorf("stop called while consumer was still active")

type consumer func(buffer string) (stopConsuming bool, err error)

type outputConsumer struct {
	_test_id string // for testing purposes only, not used for non-testing logic
	consume  consumer
	waiter   chan error
	pos      int // Todo: Find a way to move the responsibility of this entirely into outputconsumer
	opts     *OutputConsumerOpts
}

type OutputConsumerOpts struct {
	*Opts

	// Sends the full buffer each time, with the latest data appended to the end.
	// This is the full buffer as of the point in time that the consumer started listening.
	SendFullBuffer bool
	Timeout        time.Duration
}

type SetConsOpt func(o *OutputConsumerOpts)

func OptConsInherit(o *Opts) func(o *OutputConsumerOpts) {
	return func(oco *OutputConsumerOpts) {
		oco.Opts = o
	}
}

func OptConsSendFullBuffer() func(o *OutputConsumerOpts) {
	return func(oco *OutputConsumerOpts) {
		oco.SendFullBuffer = true
	}
}

func OptsConsTimeout(timeout time.Duration) func(o *OutputConsumerOpts) {
	return func(oco *OutputConsumerOpts) {
		oco.Timeout = timeout
	}
}

func newOutputConsumer(consume consumer, opts ...SetConsOpt) *outputConsumer {
	oc := &outputConsumer{
		consume: consume,
		opts: &OutputConsumerOpts{
			Opts:    NewOpts(),
			Timeout: 5 * time.Second, // Default timeout
		},
		waiter: make(chan error, 1),
	}

	for _, optSetter := range opts {
		optSetter(oc.opts)
	}

	return oc
}

// Report will consume the given buffer and will block unless wait() has been called
func (e *outputConsumer) Report(buffer []byte) (stopConsuming bool, err error) {
	stop, err := e.consume(string(buffer))
	if err != nil {
		err = fmt.Errorf("meets threw error: %w", err)
	}
	if err != nil || stop {
		// This prevents report() from blocking in case Wait() has not been called yet
		go func() {
			e.waiter <- err
		}()
	}
	return stop, err
}

// close is by definition an error condition, because it would only be called if the consumer is still active
// under normal conditions the consumer is dropped when the wait is satisfied
func (e *outputConsumer) close() {
	e.waiter <- StopPrematureError
}

func (e *outputConsumer) wait() error {
	e.opts.Logger.Println("started waiting")
	defer e.opts.Logger.Println("stopped waiting")

	select {
	case err := <-e.waiter:
		return err
	case <-time.After(e.opts.Timeout):
		return fmt.Errorf("after %s: %w", e.opts.Timeout, TimeoutError)
	}
}
