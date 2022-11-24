package termtest

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
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
}

func newOutputProducer(opts *Opts) *outputProducer {
	return &outputProducer{
		snapshot:  []byte{},
		consumers: []*outputConsumer{},
		closed:    make(chan struct{}),
		opts:      opts,
	}
}

func (o *outputProducer) Listen(r io.Reader) error {
	return o.listen(r, o.appendBuffer, producerPollInterval, producerBufferSize)
}

func (o *outputProducer) listen(r io.Reader, append func([]byte) error, interval time.Duration, size int) error {
	o.opts.Logger.Println("outputProducer listen started")
	defer o.opts.Logger.Println("outputProducer listen stopped")

	// Most of the actual logic is in listenPoll, all we're doing here is looping and signaling ready after the first
	// iteration
	for {
		select {
		case <-o.closed:
			return fmt.Errorf("outputProducer closed before EOF was reached")
		case <-time.After(interval):
			snapshot := make([]byte, size)
			n, err := r.Read(snapshot)
			if n > 0 {
				append(snapshot[:n])
			}

			// Error doesn't necessarily mean something went wrong, we may just have reached the natural end
			if err != nil {
				if errors.Is(err, fs.ErrClosed) || errors.Is(err, io.EOF) {
					o.opts.Logger.Printf(
						"Stopping reader as pty is closed or EOF reached. Buffer:\n%s\nError: %s",
						o.Snapshot(), err.Error())

					// Close outputDigester
					if err := o.Close(); err != nil {
						return fmt.Errorf("Failed to close output reader: %w", err)
					}

					return nil
				}
				return fmt.Errorf("could not read pty output: %v", err)
			}
		}
	}
}

func (o *outputProducer) appendBuffer(value []byte) error {
	o.snapshot = append(o.snapshot, value...)

	o.opts.Logger.Println("Flushing output consumers")
	defer o.opts.Logger.Println("Flushed output consumers")

	for n, consumer := range o.consumers {
		keepConsuming, err := consumer.report(o.snapshot[consumer.pos:])
		if err != nil {
			return fmt.Errorf("expectation threw error: %w", err)
		}

		consumer.pos = len(o.snapshot)

		if !keepConsuming {
			// Drop expectation
			o.consumers = append(o.consumers[:n], o.consumers[n+1:]...)
		}
	}

	return nil
}

func (o *outputProducer) Close() error {
	for _, listener := range o.consumers {
		listener.close()
	}
	close(o.closed)
	return nil
}

func (o *outputProducer) addConsumer(consume consumer, timeout time.Duration) *outputConsumer {
	listener := newOutputConsumer(consume, timeout, o.opts)
	listener.pos = len(o.snapshot)
	o.consumers = append(o.consumers, listener)
	return listener
}

func (o *outputProducer) Snapshot() []byte {
	return o.snapshot
}
