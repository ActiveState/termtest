package termtest

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sync"
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
	stop      chan struct{}
	mutex     *sync.Mutex
}

func newOutputProducer(opts *Opts) *outputProducer {
	return &outputProducer{
		snapshot:  []byte{},
		consumers: []*outputConsumer{},
		stop:      make(chan struct{}, 1),
		opts:      opts,
		mutex:     &sync.Mutex{},
	}
}

func (o *outputProducer) Listen(r io.Reader) error {
	return o.listen(r, o.appendBuffer, producerPollInterval, producerBufferSize)
}

func (o *outputProducer) listen(r io.Reader, appendBuffer func([]byte) error, interval time.Duration, size int) (rerr error) {
	o.opts.Logger.Println("listen started")
	defer func() { o.opts.Logger.Printf("listen stopped, err: %v\n", rerr) }()

	br := bufio.NewReader(r)

	// Most of the actual logic is in processNextRead, all we're doing here is looping and signaling ready after the first
	// iteration
	for {
		o.opts.Logger.Println("listen: loop")
		select {
		case <-o.stop:
			o.opts.Logger.Println("listen: closed called")
			return nil
		case <-time.After(interval):
			o.opts.Logger.Println("listen: interval called")
			if err := o.processNextRead(br, appendBuffer, size); err != nil {
				if errors.Is(err, fs.ErrClosed) || errors.Is(err, io.EOF) {
					return nil
				} else {
					return fmt.Errorf("could not poll reader: %w", err)
				}
			}
		}
	}
}

func (o *outputProducer) processNextRead(r io.Reader, appendBuffer func([]byte) error, size int) error {
	o.opts.Logger.Printf("processNextRead started with size: %d\n", size)
	defer o.opts.Logger.Println("processNextRead stopped")

	snapshot := make([]byte, size)
	n, errRead := r.Read(snapshot)
	if n > 0 {
		snapshot = cleanPtySnapshot(snapshot[:n], o.opts.Posix)
		o.opts.Logger.Printf("outputProducer read %d bytes from pty, value: %s", n, snapshot[:n])
		if err := appendBuffer(snapshot); err != nil {
			return fmt.Errorf("could not append buffer: %w", err)
		}
	}

	// Error doesn't necessarily mean something went wrong, we may just have reached the natural end
	// It's the consumers job to check for EOF errors and ignore them if they're expected
	if errRead != nil {
		return fmt.Errorf("could not read pty output: %w", errRead)
	}

	return nil
}

func (o *outputProducer) appendBuffer(value []byte) error {
	o.snapshot = append(o.snapshot, value...)

	o.opts.Logger.Printf("flushing %d output consumers", len(o.consumers))
	defer o.opts.Logger.Println("flushed output consumers")

	if err := o.flushConsumers(); err != nil {
		return fmt.Errorf("could not flush consumers: %w", err)
	}

	return nil
}

func (o *outputProducer) flushConsumers() error {
	o.opts.Logger.Println("flushing consumers")
	defer o.opts.Logger.Println("flushed consumers")

	o.mutex.Lock()
	defer o.mutex.Unlock()

	if len(o.snapshot) == 0 {
		o.opts.Logger.Println("no snapshot to flush")
		return nil
	}

	for n, consumer := range o.consumers {
		endPos, err := consumer.Report(o.snapshot)
		o.opts.Logger.Printf("consumer reported endpos: %d, err: %v", endPos, err)
		if err != nil {
			return fmt.Errorf("consumer threw error: %w", err)
		}

		if endPos > 0 {
			if endPos > len(o.snapshot) {
				return fmt.Errorf("consumer reported end position %d greater than snapshot length %d", endPos, len(o.snapshot))
			}
			o.snapshot = o.snapshot[endPos:]

			// Drop consumer
			o.opts.Logger.Printf("dropping consumer")
			o.consumers = append(o.consumers[:n], o.consumers[n+1:]...)
		}
	}

	return nil
}

func (o *outputProducer) close() error {
	o.opts.Logger.Printf("closing output producer")
	defer o.opts.Logger.Printf("closed output producer")

	for _, consumer := range o.consumers {
		// This will cause the consumer to return an error because if used correctly there shouldn't be any running
		// consumers at this time
		consumer.close()
	}

	o.opts.Logger.Printf("closing channel")
	close(o.stop)
	o.opts.Logger.Printf("channel closed")

	return nil
}

func (o *outputProducer) addConsumer(consume consumer, opts ...SetConsOpt) (*outputConsumer, error) {
	o.opts.Logger.Printf("adding consumer")
	defer o.opts.Logger.Printf("added consumer")

	opts = append(opts, OptConsInherit(o.opts))
	listener := newOutputConsumer(consume, opts...)
	o.consumers = append(o.consumers, listener)

	if len(o.snapshot) > 0 {
		if err := o.flushConsumers(); err != nil {
			return nil, fmt.Errorf("could not flush consumers: %w", err)
		}
	}

	return listener, nil
}

func (o *outputProducer) Snapshot() []byte {
	return o.snapshot
}
