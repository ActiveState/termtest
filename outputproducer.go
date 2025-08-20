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
	output       []byte
	cursorPos    int // The position of our virtual cursor, which is the position up to where we've satisfied consumers
	cleanUptoPos int // Up to which position we've cleaned the output, because incomplete output cannot be cleaned
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
			if errors.Is(err, PtyEOF) {
				return nil
			} else {
				return fmt.Errorf("could not poll reader: %w", err)
			}
		}
	}
}

var PtyEOF = errors.New("pty closed")

func (o *outputProducer) processNextRead(r io.Reader, w io.Writer, appendBuffer func([]byte, bool) error, size int) error {
	isEOF := false
	o.opts.Logger.Printf("processNextRead started with size: %d\n", size)
	defer func() {
		o.opts.Logger.Printf("processNextRead stopped, isEOF: %v\n", isEOF)
	}()

	snapshot := make([]byte, size)
	n, errRead := r.Read(snapshot)

	if errRead != nil {
		pathError := &fs.PathError{}
		if errors.Is(errRead, fs.ErrClosed) || errors.Is(errRead, io.EOF) || (runtime.GOOS == "linux" && errors.As(errRead, &pathError)) {
			isEOF = true
			o.opts.Logger.Println("reached EOF")
		}
	}

	if n > 0 {
		o.opts.Logger.Printf("outputProducer read %d bytes from pty, value: %#v", n, string(snapshot[:n]))
		if _, err := w.Write(snapshot[:n]); err != nil {
			return fmt.Errorf("could not write: %w", err)
		}
	}

	if n > 0 || isEOF {
		if err := appendBuffer(snapshot[:n], isEOF); err != nil {
			return fmt.Errorf("could not append buffer: %w", err)
		}
	}

	if errRead != nil {
		if isEOF {
			o.closeConsumers(PtyEOF)
			return errors.Join(errRead, PtyEOF)
		}
		return fmt.Errorf("could not read pty output: %w", errRead)
	}

	return nil
}

func (o *outputProducer) appendBuffer(value []byte, isFinal bool) error {
	o.opts.Logger.Printf("appendBuffer called with %d bytes, isFinal=%v", len(value), isFinal)
	if o.opts.NormalizedLineEnds {
		o.opts.Logger.Println("NormalizedLineEnds prior to appendBuffer")
		value = NormalizeLineEndsB(value)
	}

	output := append(o.output, value...)

	// Clean output
	var err error
	o.output, o.cursorPos, o.cleanUptoPos, err = o.processDirtyOutput(output, o.cursorPos, o.cleanUptoPos, isFinal, func(output []byte, cursorPos int) ([]byte, int, int, error) {
		var err error
		output, cursorPos, cleanCursorPos := cleanPtySnapshot(output, cursorPos, o.opts.Posix)
		if o.opts.OutputSanitizer != nil {
			output, cursorPos, cleanCursorPos, err = o.opts.OutputSanitizer(output, cursorPos)
		}
		return output, cursorPos, cleanCursorPos, err
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

type cleanerFunc func(snapshot []byte, cursorPos int) (newSnapshot []byte, newCursorPos int, cleanUptoPos int, err error)

// processDirtyOutput will sanitize the output received, but we have to be careful not to clean output that hasn't fully arrived
// For example we may be inside an escape sequence and the escape sequence hasn't finished
// So instead we only process new output up to the most recent line break
// In order for this to work properly the invoker must ensure the output and cleanUptoPos are consistent with each other.
func (o *outputProducer) processDirtyOutput(output []byte, cursorPos int, cleanUptoPos int, isFinal bool, cleaner cleanerFunc) (_output []byte, _cursorPos int, _cleanUptoPos int, _err error) {
	defer func() {
		o.opts.Logger.Printf("Cleaned output from %d to %d (isFinal: %v)\n", cleanUptoPos, _cleanUptoPos, isFinal)
	}()
	alreadyCleanedOutput := copyBytes(output[:cleanUptoPos])
	processedOutput := []byte{}
	unprocessedOutput := copyBytes(output[cleanUptoPos:])
	relativeCursorPos := cursorPos - len(alreadyCleanedOutput)

	// Invoke the cleaner now that we have output that can be cleaned
	newCleanUptoPos := cleanUptoPos
	if len(unprocessedOutput) > 0 {
		var err error
		var processedCleanUptoPos int
		processedOutput, relativeCursorPos, processedCleanUptoPos, err = cleaner(unprocessedOutput, relativeCursorPos)
		if err != nil {
			return processedOutput, relativeCursorPos, processedCleanUptoPos, fmt.Errorf("cleaner failed: %w", err)
		}
		// Keep a record of what point we're up to
		newCleanUptoPos += processedCleanUptoPos
	}

	// Convert cursor position back to absolute
	processedCursorPos := relativeCursorPos + len(alreadyCleanedOutput)

	if processedCursorPos < 0 {
		// Because the cleaner function needs to support a negative cursor position it is impossible for the cleaner
		// to know when they've reached the start of the output, so we need to facilitate it here.
		// Alternatively we could teach the cleaner about its absolute position, so it can handle this.
		processedCursorPos = 0
	}

	// Stitch everything back together
	return append(alreadyCleanedOutput, processedOutput...), processedCursorPos, newCleanUptoPos, nil
}

func (o *outputProducer) closeConsumers(reason error) {
	o.opts.Logger.Println("closing consumers")
	defer o.opts.Logger.Println("closed consumers")

	o.mutex.Lock()
	defer o.mutex.Unlock()

	for n := 0; n < len(o.consumers); n++ {
		o.consumers[n].Stop(reason)
		o.consumers = append(o.consumers[:n], o.consumers[n+1:]...)
	}
}

func (o *outputProducer) flushConsumers() error {
	o.opts.Logger.Println("flushing consumers")
	defer o.opts.Logger.Println("flushed consumers")

	o.mutex.Lock()
	defer o.mutex.Unlock()

	for n := 0; n < len(o.consumers); n++ {
		consumer := o.consumers[n]
		snapshot := o.PendingOutput() // o.PendingOutput() considers the cursorPos
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
			o.cursorPos += endPos

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
	return o.output[o.cursorPos:]
}

func (o *outputProducer) Output() []byte {
	return o.output
}
