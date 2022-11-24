package termtest

import (
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func Test_outputProducer_listen(t *testing.T) {
	defer goleak.VerifyNone(t)

	producerInterval := 100 * time.Millisecond
	chunkInterval := producerInterval + (time.Millisecond * 10)
	bufferSize := 10
	valExceedBuff := randString(bufferSize + 1)

	tests := []struct {
		name        string
		op          *outputProducer
		reader      io.Reader
		wantAppends []string
		wantErr     error
	}{
		{
			"Simple",
			newOutputProducer(newTestOpts(nil)),
			&readTester{
				[]readTesterChunk{
					{[]byte("One"), chunkInterval},
					{[]byte("Two"), chunkInterval},
					{[]byte("Three"), chunkInterval},
				},
			},
			[]string{"One", "Two", "Three"},
			nil,
		},
		{
			"Exceed Buffer Size",
			newOutputProducer(newTestOpts(nil)),
			&readTester{
				[]readTesterChunk{
					{[]byte(valExceedBuff), chunkInterval},
				},
			},
			[]string{valExceedBuff[:bufferSize], valExceedBuff[bufferSize:]},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := []string{}
			append := func(v []byte) error {
				got = append(got, string(v))
				return nil
			}
			if err := tt.op.listen(tt.reader, append, producerInterval, bufferSize); !errors.Is(err, tt.wantErr) {
				t.Errorf("listen() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.wantAppends) {
				t.Errorf("listen() got = %v, want %v", got, tt.wantAppends)
			}
		})
	}
}

func Test_outputProducer_appendBuffer(t *testing.T) {
	defer goleak.VerifyNone(t)

	consumerError := errors.New("consumer error")

	// consumerCalls is used to track consumer calls and their results
	// The key is the id of the consumer
	// The slice of strings are the buffer values that were passed to the consumer
	type consumerCalls map[string][]string

	// createConsumer helps reduce the boilerplate of creating a consumers
	// id is used to track which consumers are still active
	// stopAfter will cause it to send keepConsuming=false when encountering the given buffer
	// errOn will fire an error when encountering the given buffer
	// resultConsumerCalls is used to track consumer calls and their results
	createConsumer := func(id string, stopAfter string, errOn string, resultConsumerCalls consumerCalls) *outputConsumer {
		return &outputConsumer{
			_test_id: id,
			timeout:  time.Second,
			consume: func(buffer string) (keepConsuming bool, err error) {
				// Record consumer call
				if _, ok := resultConsumerCalls[id]; !ok {
					resultConsumerCalls[id] = []string{}
				}
				resultConsumerCalls[id] = append(resultConsumerCalls[id], buffer)

				// Trigger error if errOn matches
				if buffer == errOn {
					return false, consumerError
				}

				// Determine whether to keep consuming
				keepConsuming = buffer != stopAfter

				return keepConsuming, nil
			},
			opts:   newTestOpts(nil),
			closed: make(chan struct{}),
			waiter: make(chan error, 1),
		}
	}

	tests := []struct {
		name              string
		op                *outputProducer
		consumers         func(consumerCalls) []*outputConsumer
		appendCalls       []string      // the appendBuffer calls we want to make
		wantAppendErrs    []error       // the errors we expect from the append calls
		wantConsumerCalls consumerCalls // the consumer calls we expect
		wantConsumerIDs   []string      // the consumer ids we expect to be active after the test
	}{
		{
			name: "Consumer called and removed",
			op:   newOutputProducer(newTestOpts(nil)),
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
			op:   newOutputProducer(newTestOpts(nil)),
			consumers: func(resultConsumerCalls consumerCalls) []*outputConsumer {
				return []*outputConsumer{
					createConsumer("Only Consumer", "", "", resultConsumerCalls),
				}
			},
			appendCalls:    []string{"Hello"},
			wantAppendErrs: []error{nil},
			wantConsumerCalls: consumerCalls{
				"Only Consumer": {"Hello"},
			},
			wantConsumerIDs: []string{"Only Consumer"},
		},
		{
			name: "Multiple appends",
			op:   newOutputProducer(newTestOpts(nil)),
			consumers: func(resultConsumerCalls consumerCalls) []*outputConsumer {
				return []*outputConsumer{
					createConsumer("Only Consumer", "", "", resultConsumerCalls),
				}
			},
			appendCalls:    []string{"Hello", "World"},
			wantAppendErrs: []error{nil, nil},
			wantConsumerCalls: consumerCalls{
				"Only Consumer": {"Hello", "World"},
			},
			wantConsumerIDs: []string{"Only Consumer"},
		},
		{
			name: "Mixed Consumer Calls",
			op:   newOutputProducer(newTestOpts(nil)),
			consumers: func(resultConsumerCalls consumerCalls) []*outputConsumer {
				return []*outputConsumer{
					createConsumer("Removed Consumer", "Hello", "", resultConsumerCalls),
					createConsumer("Kept Consumer", "", "", resultConsumerCalls),
				}
			},
			appendCalls:    []string{"Hello"},
			wantAppendErrs: []error{nil},
			wantConsumerCalls: consumerCalls{
				"Removed Consumer": {"Hello"},
				"Kept Consumer":    {"Hello"},
			},
			wantConsumerIDs: []string{"Kept Consumer"},
		},
		{
			name: "Mixed Consumer Calls with multiple appends",
			op:   newOutputProducer(newTestOpts(nil)),
			consumers: func(resultConsumerCalls consumerCalls) []*outputConsumer {
				return []*outputConsumer{
					createConsumer("Removed Consumer", "Two", "", resultConsumerCalls),
					createConsumer("Kept Consumer", "", "", resultConsumerCalls),
				}
			},
			appendCalls:    []string{"One", "Two", "Three"},
			wantAppendErrs: []error{nil, nil, nil},
			wantConsumerCalls: consumerCalls{
				"Removed Consumer": {"One", "Two"},
				"Kept Consumer":    {"One", "Two", "Three"},
			},
			wantConsumerIDs: []string{"Kept Consumer"},
		},
		{
			name: "Consumer error",
			op:   newOutputProducer(newTestOpts(nil)),
			consumers: func(resultConsumerCalls consumerCalls) []*outputConsumer {
				return []*outputConsumer{
					createConsumer("Only Consumer", "", "Hello", resultConsumerCalls),
				}
			},
			appendCalls:    []string{"Hello"},
			wantAppendErrs: []error{consumerError},
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

			resultConsumerCalls := consumerCalls{}
			tt.op.consumers = tt.consumers(resultConsumerCalls)

			for _, consumer := range tt.op.consumers {
				go consumer.Wait() // Otherwise appendBuffer will block
			}

			for n, append := range tt.appendCalls {
				if err := tt.op.appendBuffer([]byte(append)); !errors.Is(err, tt.wantAppendErrs[n]) {
					t.Errorf("appendBuffer() error = %v, wantErr %v", err, tt.wantAppendErrs[n])
				}
			}
			if !reflect.DeepEqual(resultConsumerCalls, tt.wantConsumerCalls) {
				t.Errorf("resultConsumerCalls = %v, want %v", resultConsumerCalls, tt.wantConsumerCalls)
			}

			gotConsumerIDs := []string{}
			for _, consumer := range tt.op.consumers {
				gotConsumerIDs = append(gotConsumerIDs, consumer._test_id)
			}
			if !reflect.DeepEqual(gotConsumerIDs, tt.wantConsumerIDs) {
				t.Errorf("gotConsumerIDs = %v, want %v", gotConsumerIDs, tt.wantConsumerIDs)
			}
		})
	}
}
