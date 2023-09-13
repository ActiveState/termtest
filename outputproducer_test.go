package termtest

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_outputProducer_listen(t *testing.T) {
	producerInterval := 100 * time.Millisecond
	chunkInterval := producerInterval + (time.Millisecond * 10)
	bufferSize := 10
	valExceedBuff := randString(bufferSize + 1)

	tests := []struct {
		name        string
		op          func(t *testing.T) *outputProducer
		reader      io.Reader
		wantAppends []string
		wantErr     error
	}{
		{
			"Simple",
			func(t *testing.T) *outputProducer { return newOutputProducer(newTestOpts(nil, t)) },
			&readTester{
				[]readTesterChunk{
					{[]byte("One"), chunkInterval},
					{[]byte("Two"), chunkInterval},
					{[]byte("Three"), chunkInterval},
				},
			},
			[]string{"One", "Two", "Three", ""}, // Last append is EOF and should ALWAYS happen
			nil,
		},
		{
			"Exceed Buffer Size",
			func(t *testing.T) *outputProducer { return newOutputProducer(newTestOpts(nil, t)) },
			&readTester{
				[]readTesterChunk{
					{[]byte(valExceedBuff), chunkInterval},
				},
			},
			[]string{valExceedBuff[:bufferSize], valExceedBuff[bufferSize:], ""}, // Last append is EOF and should ALWAYS happen
			nil,
		},
		{
			"EOF should trigger an append, even if new output is empty",
			func(t *testing.T) *outputProducer { return newOutputProducer(newTestOpts(nil, t)) },
			&readTester{
				[]readTesterChunk{
					{[]byte("foo"), chunkInterval},
				},
			},
			[]string{"foo", ""},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := []string{}
			appendV := func(v []byte, _ bool) error {
				got = append(got, string(v))
				return nil
			}
			op := tt.op(t)
			err := op.listen(tt.reader, &BlackholeWriter{}, appendV, producerInterval, bufferSize)
			if errors.Is(err, io.EOF) {
				err = nil
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("listen() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.wantAppends) {
				t.Errorf("listen() got = %v, want %v", got, tt.wantAppends)
			}
		})
	}
}

func Test_outputProducer_appendBuffer(t *testing.T) {
	consumerError := errors.New("consumer error")

	// consumerCalls is used to track consumer calls and their results
	// The key is the id of the consumer
	// The slice of strings are the buffer values that were passed to the consumer
	type consumerCalls map[string][]string

	type consumerWaitErrs map[string]error

	// createConsumer helps reduce the boilerplate of creating a consumers
	// id is used to track which consumers are still active
	// stopAfter will cause it to send stopConsuming=true when encountering the given buffer
	// errOn will fire an error when encountering the given buffer
	// resultConsumerCalls is used to track consumer calls and their results
	createConsumer := func(id string, stopAfter string, errOn string, resultConsumerCalls consumerCalls, opts ...SetConsOpt) *outputConsumer {
		consumer := func(buffer string) (endPos int, err error) {
			// Record consumer call
			if _, ok := resultConsumerCalls[id]; !ok {
				resultConsumerCalls[id] = []string{}
			}
			resultConsumerCalls[id] = append(resultConsumerCalls[id], buffer)

			// Trigger error if errOn matches
			if buffer == errOn {
				return 0, consumerError
			}

			if i := strings.Index(buffer, stopAfter); i != -1 {
				return i + len(stopAfter), nil
			}

			return 0, nil
		}
		oc := newOutputConsumer(consumer, append(opts, OptsConsTimeout(time.Second))...)
		oc._test_id = id
		return oc
	}

	tests := []struct {
		name              string
		op                func(t *testing.T) *outputProducer
		consumers         func(consumerCalls) []*outputConsumer
		appendCalls       []string         // the appendBuffer calls we want to make
		wantAppendErrs    []error          // the errors we expect from the append calls
		wantWaitErrs      consumerWaitErrs // the error we expect from the wait call
		wantConsumerCalls consumerCalls    // the consumer calls we expect
		wantConsumerIDs   []string         // the consumer ids we expect to be active after the test
	}{
		{
			name: "Consumer called and removed",
			op:   func(t *testing.T) *outputProducer { return newOutputProducer(newTestOpts(nil, t)) },
			consumers: func(resultConsumerCalls consumerCalls) []*outputConsumer {
				return []*outputConsumer{
					createConsumer("Only Consumer", "Hello", "", resultConsumerCalls),
				}
			},
			appendCalls:    []string{"Hello"},
			wantAppendErrs: []error{nil},
			wantConsumerCalls: consumerCalls{
				"Only Consumer": {"Hello"},
			},
			wantConsumerIDs: []string{},
		},
		{
			name: "Consumer called and remained",
			op:   func(t *testing.T) *outputProducer { return newOutputProducer(newTestOpts(nil, t)) },
			consumers: func(resultConsumerCalls consumerCalls) []*outputConsumer {
				return []*outputConsumer{
					createConsumer("Only Consumer", "", "", resultConsumerCalls),
				}
			},
			appendCalls:    []string{"Hello"},
			wantAppendErrs: []error{nil},
			wantWaitErrs: consumerWaitErrs{
				"Only Consumer": TimeoutError,
			},
			wantConsumerCalls: consumerCalls{
				"Only Consumer": {"Hello"},
			},
			wantConsumerIDs: []string{"Only Consumer"},
		},
		{
			name: "Multiple appends",
			op:   func(t *testing.T) *outputProducer { return newOutputProducer(newTestOpts(nil, t)) },
			consumers: func(resultConsumerCalls consumerCalls) []*outputConsumer {
				return []*outputConsumer{
					createConsumer("Only Consumer", "", "", resultConsumerCalls),
				}
			},
			appendCalls:    []string{"Hello", "World"},
			wantAppendErrs: []error{nil, nil},
			wantWaitErrs: consumerWaitErrs{
				"Only Consumer": TimeoutError,
			},
			wantConsumerCalls: consumerCalls{
				"Only Consumer": {"Hello", "HelloWorld"},
			},
			wantConsumerIDs: []string{"Only Consumer"},
		},
		{
			name: "Mixed Consumer Calls",
			op:   func(t *testing.T) *outputProducer { return newOutputProducer(newTestOpts(nil, t)) },
			consumers: func(resultConsumerCalls consumerCalls) []*outputConsumer {
				return []*outputConsumer{
					createConsumer("Kept Consumer", "", "", resultConsumerCalls),
					createConsumer("Removed Consumer", "Hello", "", resultConsumerCalls),
				}
			},
			appendCalls:    []string{"Hello"},
			wantAppendErrs: []error{nil},
			wantWaitErrs: consumerWaitErrs{
				"Kept Consumer":    TimeoutError,
				"Removed Consumer": nil,
			},
			wantConsumerCalls: consumerCalls{
				"Removed Consumer": {"Hello"},
				"Kept Consumer":    {"Hello"},
			},
			wantConsumerIDs: []string{"Kept Consumer"},
		},
		{
			name: "Mixed Consumer Calls with multiple appends",
			op:   func(t *testing.T) *outputProducer { return newOutputProducer(newTestOpts(nil, t)) },
			consumers: func(resultConsumerCalls consumerCalls) []*outputConsumer {
				return []*outputConsumer{
					createConsumer("Kept Consumer", "", "", resultConsumerCalls),
					createConsumer("Removed Consumer", "Two", "", resultConsumerCalls),
				}
			},
			appendCalls:    []string{"One", "Two", "Three"},
			wantAppendErrs: []error{nil, nil, nil},
			wantWaitErrs: consumerWaitErrs{
				"Kept Consumer":    TimeoutError,
				"Removed Consumer": nil,
			},
			wantConsumerCalls: consumerCalls{
				"Removed Consumer": {"One", "OneTwo"},
				"Kept Consumer":    {"One", "OneTwo", "Three" /* Removed consumer matched "Two", so the buffer has moved on */},
			},
			wantConsumerIDs: []string{"Kept Consumer"},
		},
		{
			name: "Consumer error",
			op:   func(t *testing.T) *outputProducer { return newOutputProducer(newTestOpts(nil, t)) },
			consumers: func(resultConsumerCalls consumerCalls) []*outputConsumer {
				return []*outputConsumer{
					createConsumer("Only Consumer", "", "Hello", resultConsumerCalls),
				}
			},
			appendCalls:    []string{"Hello"},
			wantAppendErrs: []error{consumerError},
			wantWaitErrs: consumerWaitErrs{
				"Only Consumer": consumerError,
			},
			wantConsumerCalls: consumerCalls{
				"Only Consumer": {"Hello"},
			},
			wantConsumerIDs: []string{"Only Consumer"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.appendCalls) != len(tt.wantAppendErrs) {
				t.Fatalf("appendCalls and wantAppendErrs must be same length")
			}

			op := tt.op(t)
			resultConsumerCalls := consumerCalls{}
			op.consumers = tt.consumers(resultConsumerCalls)

			wg := &sync.WaitGroup{}
			if tt.wantWaitErrs != nil {
				for _, consumer := range op.consumers {
					wg.Add(1)
					go func(consumer *outputConsumer) { // Otherwise appendBuffer will block
						defer wg.Done()
						err := consumer.wait()
						require.ErrorIs(t, err, tt.wantWaitErrs[consumer._test_id],
							"consumer %s expected error %T, got %T", consumer._test_id, tt.wantWaitErrs[consumer._test_id], err)
					}(consumer)
				}
			}

			for n, appendV := range tt.appendCalls {
				if err := op.appendBuffer([]byte(appendV), false); !errors.Is(err, tt.wantAppendErrs[n]) {
					t.Errorf("appendBuffer() error = %v, wantErr %v", err, tt.wantAppendErrs[n])
				}
			}

			wg.Wait()

			if !reflect.DeepEqual(resultConsumerCalls, tt.wantConsumerCalls) {
				t.Errorf("resultConsumerCalls = %v, want %v", resultConsumerCalls, tt.wantConsumerCalls)
			}

			gotConsumerIDs := []string{}
			for _, consumer := range op.consumers {
				gotConsumerIDs = append(gotConsumerIDs, consumer._test_id)
			}
			if !reflect.DeepEqual(gotConsumerIDs, tt.wantConsumerIDs) {
				t.Errorf("gotConsumerIDs = %v, want %v", gotConsumerIDs, tt.wantConsumerIDs)
			}
		})
	}
}

func Test_outputProducer_cleanOutput(t *testing.T) {
	phraseSanitizer := func(b []byte, cursorPos int) ([]byte, int, error) {
		return regexp.MustCompile(`([\w ]+)`).ReplaceAll(b, []byte("sanitized")), cursorPos, nil
	}
	inc := 0
	incrementalPhraseSanitizer := func(b []byte, cursorPos int) ([]byte, int, error) {
		defer func() { inc++ }()
		return regexp.MustCompile(`([\w ]+)`).ReplaceAll(b, []byte(fmt.Sprintf("sanitized%d", inc))), cursorPos, nil
	}

	type invocation struct {
		appendBytes            []byte
		isFinal                bool
		wantOutput             []byte
		wantRelCursorPosInput  int // The cursor position sent into the cleaner
		wantRelCursorPosOutput int // The cursor position returned by the cleaner
		wantAbsCursorPos       int
		wantAbsCleanerPos      int
		wantErr                require.ErrorAssertionFunc
	}
	tests := []struct {
		name        string
		op          *outputProducer
		cursorPos   int
		cleanerPos  int
		cleaner     cleanerFunc
		invocations []invocation
	}{
		{
			"Do not sanitize unfinished",
			&outputProducer{},
			0,
			0,
			func([]byte, int) ([]byte, int, error) {
				return []byte(""), -1, fmt.Errorf("I should not have been invoked")
			},
			[]invocation{
				{
					[]byte("not final cause I dont have a line end"),
					false,
					[]byte("not final cause I dont have a line end"),
					0,
					0,
					0,
					0,
					require.NoError,
				},
			},
		},
		{
			"Sanitize finished",
			&outputProducer{},
			0,
			0,
			func([]byte, int) ([]byte, int, error) {
				return []byte("sanitized"), 0, nil
			},
			[]invocation{
				{
					[]byte("final cause I'm sending isFinal=true'"),
					true,
					[]byte("sanitized"),
					0,
					0,
					0,
					9,
					require.NoError,
				},
			},
		},
		{
			"Sanitize up to final line end",
			&outputProducer{},
			0,
			0,
			phraseSanitizer,
			[]invocation{
				{
					[]byte("sanitize\nsanitize\ndont sanitize"),
					false,
					[]byte("sanitized\nsanitized\ndont sanitize"),
					0,
					0,
					0,
					20,
					require.NoError,
				},
			},
		},
		{
			"Sanitize from pos up to final line end",
			&outputProducer{},
			0,
			21,
			phraseSanitizer,
			[]invocation{
				{
					[]byte("previously sanitized\nsanitize\ndont sanitize"),
					false,
					[]byte("previously sanitized\nsanitized\ndont sanitize"),
					-21,
					-21,
					0,
					31,
					require.NoError,
				},
			},
		},
		{
			"Consecutive Invocations",
			&outputProducer{},
			0,
			0,
			incrementalPhraseSanitizer,
			[]invocation{
				{
					// This won't result in anything being sanitized, because isFinal=false and there is no line end
					[]byte("sanitize me"),
					false,
					[]byte("sanitize me"),
					0,
					0,
					0,
					0,
					require.NoError,
				},
				{
					// The new text won't  get sanitized, but because we're adding a line break at the start here which
					// will get appended to the previous invocation we should now get the bytes produced by the previous
					// invocation sanitized.
					[]byte("\nsanitize me"),
					false,
					[]byte("sanitized0\nsanitize me"),
					0,
					0,
					0,
					11,
					require.NoError,
				},
				{
					// We're just appending a new line end here, so all output produced up to this point should now get
					// sanitized. The integer at the end of the sanitized output lets us know which invocation it got
					// sanitized on.
					[]byte("\n"),
					false,
					[]byte("sanitized0\nsanitized1\n"),
					-11,
					-11,
					0,
					22,
					require.NoError,
				},
				{
					// No line end on the new output, but we're sending isFinal=true, so the output should be sanitized
					[]byte("sanitize me"),
					true,
					[]byte("sanitized0\nsanitized1\nsanitized2"),
					-22,
					-22,
					0,
					32,
					require.NoError,
				},
			},
		},
		{
			"Cursor Movement",
			&outputProducer{},
			27, // Space before "me" in "sanitize me"
			18, // End of first linebreak
			func([]byte, int) ([]byte, int, error) {
				return []byte("sanitized\n"), 0, nil
			},
			[]invocation{
				{
					[]byte("already sanitized\nsanitize me\n"),
					false,
					[]byte("already sanitized\nsanitized\n"),
					9,
					0,
					18,
					28,
					require.NoError,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := tt.op
			cleanerPos := tt.cleanerPos
			cursorPos := tt.cursorPos
			output := []byte{}
			for n, inv := range tt.invocations {
				cont := t.Run(fmt.Sprintf("%d", n), func(t *testing.T) {
					output = append(output, inv.appendBytes...)
					got, newCursorPos, newCleanerPos, err := o.processDirtyOutput(
						output, cursorPos, cleanerPos, inv.isFinal,
						func(output []byte, cursorPos int) ([]byte, int, error) {
							require.Equal(t, inv.wantRelCursorPosInput, cursorPos)
							out, newCursorPos, err := tt.cleaner(output, cursorPos)
							require.Equal(t, inv.wantRelCursorPosOutput, newCursorPos)
							return out, newCursorPos, err
						})

					inv.wantErr(t, err)

					require.Equal(t, string(inv.wantOutput), string(got))
					require.Equal(t, inv.wantAbsCursorPos, newCursorPos)
					require.Equal(t, inv.wantAbsCleanerPos, newCleanerPos)

					cleanerPos = newCleanerPos
					cursorPos = newCursorPos
					output = got
				})
				if !cont {
					break
				}
			}
		})
	}
}
