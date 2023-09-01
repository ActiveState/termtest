package termtest

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"runtime"
	"sync"
	"time"
)

// producerPollInterval is the interval at which the output producer will poll the pty for new output
const producerPollInterval = 100 * time.Millisecond

// producerBufferSize is the maximum size of the snapshot buffer that we read on each interval
const producerBufferSize = 1024

// outputProducer is responsible for keeping track of the output and notifying consumers when new output is produced
type outputProducer struct {
	output      []byte
	snapshotPos int
	consumers   []*outputConsumer
	opts        *Opts
	mutex       *sync.Mutex
}

func newOutputProducer(opts *Opts) *outputProducer {
	return &outputProducer{
		output:    []byte{},
		consumers: []*outputConsumer{},
		opts:      opts,
		mutex:     &sync.Mutex{},
	}
}

func (o *outputProducer) Listen(r io.Reader, w io.Writer) error {
	return o.listen(r, w, o.appendBuffer, producerPollInterval, producerBufferSize)
}

func (o *outputProducer) listen(r io.Reader, w io.Writer, appendBuffer func([]byte) error, interval time.Duration, size int) (rerr error) {
	o.opts.Logger.Println("listen started")
	defer func() {
		o.opts.Logger.Printf("listen stopped, err: %v\n", rerr)
	}()

	br := bufio.NewReader(r)

	// Most of the actual logic is in processNextRead, all we're doing here is looping and signaling ready after the first
	// iteration
	for {
		o.opts.Logger.Println("listen: loop")
		if err := o.processNextRead(br, w, appendBuffer, size); err != nil {
			if errors.Is(err, ptyEOF) {
				o.opts.Logger.Println("listen: reached EOF")
				return nil
			} else {
				return fmt.Errorf("could not poll reader: %w", err)
			}
		}
	}
}

var ptyEOF = errors.New("pty closed")

func (o *outputProducer) processNextRead(r io.Reader, w io.Writer, appendBuffer func([]byte) error, size int) error {
	o.opts.Logger.Printf("processNextRead started with size: %d\n", size)
	defer o.opts.Logger.Println("processNextRead stopped")

	snapshot := make([]byte, size)
	n, errRead := r.Read(snapshot)
	if n > 0 {
		o.opts.Logger.Printf("outputProducer read %d bytes from pty, value: %s", n, snapshot[:n])
		if _, err := w.Write(snapshot[:n]); err != nil {
			return fmt.Errorf("could not write: %w", err)
		}
		snapshot = cleanPtySnapshot(snapshot[:n], o.opts.Posix)
		if err := appendBuffer(snapshot); err != nil {
			return fmt.Errorf("could not append buffer: %w", err)
		}
	}

	if errRead != nil {
		pathError := &fs.PathError{}
		if errors.Is(errRead, fs.ErrClosed) || errors.Is(errRead, io.EOF) || (runtime.GOOS == "linux" && errors.As(errRead, &pathError)) {
			return errors.Join(errRead, ptyEOF)
		}
		return fmt.Errorf("could not read pty output: %w", errRead)
	}

	return nil
}

func (o *outputProducer) appendBuffer(value []byte) error {
	output := append(o.output, value...)

	if o.opts.OutputSanitizer != nil {
		v, err := o.opts.OutputSanitizer(output)
		if err != nil {
			return fmt.Errorf("could not sanitize output: %w", err)
		}
		output = v
	}

	if o.opts.NormalizedLineEnds {
		o.opts.Logger.Println("NormalizedLineEnds prior to appendBuffer")
		output = NormalizeLineEndsB(output)
	}

	o.output = output

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

	for n := 0; n < len(o.consumers); n++ {
		consumer := o.consumers[n]
		snapshot := o.PendingOutput() // o.PendingOutput() considers the snapshotPos
		if len(snapshot) == 0 {
			o.opts.Logger.Println("no snapshot to flush")
			return nil
		}

		if !consumer.IsAlive() {
			o.opts.Logger.Printf("dropping consumer %d out of %d as it is no longer alive", n, len(o.consumers))
			o.consumers = append(o.consumers[:n], o.consumers[n+1:]...)
			n--
			continue
		}

		endPos, err := consumer.Report(snapshot)
		o.opts.Logger.Printf("consumer reported endpos: %d, err: %v", endPos, err)
		if err != nil {
			return fmt.Errorf("consumer threw error: %w", err)
		}

		if endPos > 0 {
			if endPos > len(snapshot) {
				return fmt.Errorf("consumer reported end position %d greater than snapshot length %d", endPos, len(o.output))
			}
			o.snapshotPos += endPos

			// Drop consumer
			o.opts.Logger.Printf("dropping consumer %d out of %d", n+1, len(o.consumers))
			o.consumers = append(o.consumers[:n], o.consumers[n+1:]...)
			n--
		}
	}

	return nil
}

func (o *outputProducer) addConsumer(consume consumer, opts ...SetConsOpt) (*outputConsumer, error) {
	o.opts.Logger.Printf("adding consumer")
	defer o.opts.Logger.Printf("added consumer")

	opts = append(opts, OptConsInherit(o.opts))
	listener := newOutputConsumer(consume, opts...)
	o.consumers = append(o.consumers, listener)

	if err := o.flushConsumers(); err != nil {
		return nil, fmt.Errorf("could not flush consumers: %w", err)
	}

	return listener, nil
}

func (o *outputProducer) PendingOutput() []byte {
	return o.output[o.snapshotPos:]
}

func (o *outputProducer) Output() []byte {
	return o.output
}
