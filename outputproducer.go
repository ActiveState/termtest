package termtest

import (
	"fmt"
	"io"
	"time"
)

// producerPollInterval is the interval at which the output producer will poll the pty for new output
const producerPollInterval = 100 * time.Millisecond

// producerBufferSize is the maximum size of the snapshot buffer that we read on each interval
const producerBufferSize = 1024

// outputProducer is responsible for keeping track of the output and notifying consumers when new output is produced
type outputProducer struct {
	snapshot  []byte
	consumers []*outputConsumer
	opts      *Opts
	closed    chan struct{}
	flush     chan struct{}
}

func newOutputProducer(opts *Opts) *outputProducer {
	return &outputProducer{
		snapshot:  []byte{},
		consumers: []*outputConsumer{},
		flush:     make(chan struct{}, 1),
		closed:    make(chan struct{}),
		opts:      opts,
	}
}

func (o *outputProducer) Listen(r io.Reader) error {
	return o.listen(r, o.appendBuffer, producerPollInterval, producerBufferSize)
}

func (o *outputProducer) listen(r io.Reader, appendBuffer func([]byte) error, interval time.Duration, size int) error {
	o.opts.Logger.Println("listen started")
	defer o.opts.Logger.Println("listen stopped")

	// Most of the actual logic is in listenPoll, all we're doing here is looping and signaling ready after the first
	// iteration
	for {
		select {
		case <-o.closed:
			if len(o.consumers) > 0 {
				return fmt.Errorf("outputProducer closed before consumers were satisfied")
			}
			return nil
		case <-o.flush:
			if len(o.consumers) == 0 {
				return nil
			}
			if err := o.pollReader(r, appendBuffer, size); err != nil {
				return err
			}
		case <-time.After(interval):
			if err := o.pollReader(r, appendBuffer, size); err != nil {
				return err
			}
		}
	}
}

func (o *outputProducer) pollReader(r io.Reader, appendBuffer func([]byte) error, size int) error {
	snapshot := make([]byte, size)
	n, err := r.Read(snapshot)
	if n > 0 {
		o.opts.Logger.Printf("outputProducer read %d bytes from pty", n)
		appendBuffer(snapshot[:n])
	}

	// Error doesn't necessarily mean something went wrong, we may just have reached the natural end
	// It's the consumers job to check for EOF errors and ignore them if they're expected
	if err != nil {
		return fmt.Errorf("could not read pty output: %w", err)
	}

	return nil
}

func (o *outputProducer) appendBuffer(value []byte) error {
	o.snapshot = append(o.snapshot, value...)

	o.opts.Logger.Printf("flushing %d output consumers", len(o.consumers))
	defer o.opts.Logger.Println("flushed output consumers")

	for n, consumer := range o.consumers {
		stopConsuming, err := consumer.Report(o.snapshot[consumer.pos:])
		o.opts.Logger.Printf("consumer reported stop: %v, err: %v", stopConsuming, err)
		if err != nil {
			return fmt.Errorf("consumer threw error: %w", err)
		}

		if !consumer.opts.SendFullBuffer {
			consumer.pos = len(o.snapshot)
		}

		if stopConsuming {
			o.opts.Logger.Printf("dropping consumer")

			// Drop expectation
			o.consumers = append(o.consumers[:n], o.consumers[n+1:]...)
		}
	}

	return nil
}

func (o *outputProducer) close() error {
	o.opts.Logger.Printf("closing")
	defer o.opts.Logger.Printf("closed")

	o.flush <- struct{}{}

	for _, listener := range o.consumers {
		// This will cause the consumer to return an error because if used correctly there shouldn't be any running
		// consumers at this time
		listener.close()
	}

	close(o.closed)

	return nil
}

func (o *outputProducer) addConsumer(consume consumer, timeout time.Duration, opts ...SetConsOpt) (*outputConsumer, error) {
	o.opts.Logger.Printf("adding consumer with timeout %s", timeout)

	opts = append(opts, OptInherit(o.opts))
	listener := newOutputConsumer(consume, timeout, opts...)
	listener.pos = len(o.snapshot)
	o.consumers = append(o.consumers, listener)

	return listener, nil
}

func (o *outputProducer) Snapshot() []byte {
	return o.snapshot
}
