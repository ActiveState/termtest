package termtest

import (
	"bufio"
	"bytes"
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
	output       []byte
	snapshotPos  int
	cleanUptoPos int
	consumers    []*outputConsumer
	opts         *Opts
	mutex        *sync.Mutex
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

func (o *outputProducer) listen(r io.Reader, w io.Writer, appendBuffer func([]byte, bool) error, interval time.Duration, size int) (rerr error) {
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

func (o *outputProducer) processNextRead(r io.Reader, w io.Writer, appendBuffer func([]byte, bool) error, size int) error {
	o.opts.Logger.Printf("processNextRead started with size: %d\n", size)
	defer o.opts.Logger.Println("processNextRead stopped")

	snapshot := make([]byte, size)
	n, errRead := r.Read(snapshot)

	isEOF := false
	if errRead != nil {
		pathError := &fs.PathError{}
		if errors.Is(errRead, fs.ErrClosed) || errors.Is(errRead, io.EOF) || (runtime.GOOS == "linux" && errors.As(errRead, &pathError)) {
			isEOF = true
		}
	}

	if n > 0 {
		o.opts.Logger.Printf("outputProducer read %d bytes from pty, value: %s", n, snapshot[:n])
		if _, err := w.Write(snapshot[:n]); err != nil {
			return fmt.Errorf("could not write: %w", err)
		}
		if err := appendBuffer(snapshot[:n], isEOF); err != nil {
			return fmt.Errorf("could not append buffer: %w", err)
		}
	}

	if errRead != nil {
		if isEOF {
			return errors.Join(errRead, ptyEOF)
		}
		return fmt.Errorf("could not read pty output: %w", errRead)
	}

	return nil
}

func (o *outputProducer) appendBuffer(value []byte, isFinal bool) error {
	if o.opts.NormalizedLineEnds {
		o.opts.Logger.Println("NormalizedLineEnds prior to appendBuffer")
		value = NormalizeLineEndsB(value)
	}

	output := append(o.output, value...)

	// Clean output
	var err error
	o.output, o.cleanUptoPos, err = o.processDirtyOutput(output, o.cleanUptoPos, isFinal, func(output []byte) ([]byte, error) {
		var err error
		output = cleanPtySnapshot(output, o.opts.Posix)
		if o.opts.OutputSanitizer != nil {
			output, err = o.opts.OutputSanitizer(output)
		}
		return output, err
	})
	if err != nil {
		return fmt.Errorf("cleaning output failed: %w", err)
	}

	o.opts.Logger.Printf("flushing %d output consumers", len(o.consumers))
	defer o.opts.Logger.Println("flushed output consumers")

	if err := o.flushConsumers(); err != nil {
		return fmt.Errorf("could not flush consumers: %w", err)
	}

	return nil
}

// processDirtyOutput will sanitize the output received, but we have to be careful not to clean output that hasn't fully arrived
// For example we may be inside an escape sequence and the escape sequence hasn't finished
// So instead we only process new output up to the most recent line break
// In order for this to work properly the invoker must ensure the output and cleanUptoPos are consistent with each other.
func (o *outputProducer) processDirtyOutput(output []byte, cleanUptoPos int, isFinal bool, cleaner func([]byte) ([]byte, error)) ([]byte, int, error) {
	alreadyCleanedOutput := copyBytes(output[:cleanUptoPos])
	processedOutput := []byte{}
	unprocessedOutput := copyBytes(output[cleanUptoPos:])

	if isFinal {
		// If we've reached the end there's no point looking for the most recent line break as there's no guarantee the
		// output will be terminated by a newline.
		processedOutput = copyBytes(unprocessedOutput)
		unprocessedOutput = []byte{}
	} else {
		// Find the most recent line break, and only clean until that point.
		// Any output after the most recent line break is considered not ready for cleaning as cleaning depends on
		// multiple consecutive characters.
		lineSepN := bytes.LastIndex(unprocessedOutput, []byte("\n"))
		if lineSepN != -1 {
			processedOutput = copyBytes(unprocessedOutput[0 : lineSepN+1])
			unprocessedOutput = unprocessedOutput[lineSepN+1:]
		}
	}

	// Invoke the cleaner now that we have output that can be cleaned
	if len(processedOutput) > 0 {
		var err error
		processedOutput, err = cleaner(processedOutput)
		if err != nil {
			return processedOutput, cleanUptoPos, fmt.Errorf("cleaner failed: %w", err)
		}
	}

	// Keep a record of what point we're up to
	cleanUptoPos = cleanUptoPos + len(processedOutput)

	// Stitch everything back together
	return append(append(alreadyCleanedOutput, processedOutput...), unprocessedOutput...), cleanUptoPos, nil
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
