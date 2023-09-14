package termtest

import (
	"fmt"
	"sync"
	"time"
)

type consumer func(buffer string) (matchEndPos int, err error)

type outputConsumer struct {
	_test_id string // for testing purposes only, not used for non-testing logic
	consume  consumer
	waiter   chan error
	opts     *OutputConsumerOpts
	isalive  bool
	mutex    *sync.Mutex
}

type OutputConsumerOpts struct {
	*Opts
	Timeout time.Duration
}

type SetConsOpt func(o *OutputConsumerOpts)

func OptConsInherit(o *Opts) func(o *OutputConsumerOpts) {
	return func(oco *OutputConsumerOpts) {
		oco.Opts = o
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
		waiter:  make(chan error, 1),
		isalive: true,
		mutex:   &sync.Mutex{},
	}

	for _, optSetter := range opts {
		optSetter(oc.opts)
	}

	return oc
}

func (e *outputConsumer) IsAlive() bool {
	return e.isalive
}

// Report will consume the given buffer and will block unless wait() has been called
func (e *outputConsumer) Report(buffer []byte) (int, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	pos, err := e.consume(string(buffer))
	if err != nil {
		err = fmt.Errorf("consumer threw error: %w", err)
	}
	if err == nil && pos > len(buffer) {
		err = fmt.Errorf("consumer returned endPos %d which is greater than buffer length %d", pos, len(buffer))
	}
	if err != nil || pos > 0 {
		e.opts.Logger.Printf("closing waiter from report, err: %v, endPos: %d\n", err, pos)
		// This prevents report() from blocking in case wait() has not been called yet
		go func() {
			e.waiter <- err
		}()
	}
	return pos, err
}

func (e *outputConsumer) wait() error {
	e.opts.Logger.Println("started waiting")
	defer e.opts.Logger.Println("stopped waiting")

	defer func() {
		e.isalive = false
		e.mutex.Unlock()
	}()

	select {
	case err := <-e.waiter:
		e.mutex.Lock()
		if err != nil {
			e.opts.Logger.Printf("Encountered error: %s\n", err.Error())
		}
		return err
	case <-time.After(e.opts.Timeout):
		e.mutex.Lock()
		e.opts.Logger.Println("Encountered timeout")
		return fmt.Errorf("after %s: %w", e.opts.Timeout, TimeoutError)
	}
}
