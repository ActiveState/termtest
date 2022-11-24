package termtest

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_OutputConsumer(t *testing.T) {
	var testConsumerError = errors.New("Test consumer error")

	tests := []struct {
		name          string
		oc            func(*[]string) *outputConsumer
		reports       []string
		wantReports   []string
		wantReportErr error
		wantWaitErr   error
	}{
		{
			"Simple",
			func(reports *[]string) *outputConsumer {
				return newOutputConsumer(func(buffer string) (keepConsuming bool, err error) {
					*reports = append(*reports, buffer)
					return false, nil
				}, 5*time.Second, newTestOpts(nil))
			},
			[]string{"Report"},
			[]string{"Report"},
			nil,
			nil,
		},
		{
			"Multiple reports",
			func(reports *[]string) *outputConsumer {
				return newOutputConsumer(func(buffer string) (keepConsuming bool, err error) {
					*reports = append(*reports, buffer)
					return buffer == "Three", nil
				}, 5*time.Second, newTestOpts(nil))
			},
			[]string{"One", "Two", "Three"},
			[]string{"One", "Two", "Three"},
			nil,
			nil,
		},
		{
			"Consumer error",
			func(reports *[]string) *outputConsumer {
				return newOutputConsumer(func(buffer string) (keepConsuming bool, err error) {
					return false, testConsumerError
				}, 5*time.Second, newTestOpts(nil))
			},
			[]string{"Report"},
			[]string{},
			testConsumerError,
			testConsumerError,
		},
		{
			"Premature close",
			func(reports *[]string) *outputConsumer {
				oc := newOutputConsumer(func(buffer string) (keepConsuming bool, err error) {
					*reports = append(*reports, buffer)
					return false, testConsumerError
				}, 5*time.Second, newTestOpts(nil))
				oc.close()
				return oc
			},
			[]string{"Report"},
			[]string{},
			StoppedError,
			StopPrematureError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReports := []string{}
			oc := tt.oc(&gotReports)

			wg := &sync.WaitGroup{}
			wg.Add(1)
			go func() {
				for _, report := range tt.reports {
					_, err := oc.report([]byte(report))
					assert.ErrorIs(t, err, tt.wantReportErr)
				}
				wg.Done()
			}()

			err := oc.Wait()
			assert.ErrorIs(t, err, tt.wantWaitErr)

			wg.Wait()

			if !reflect.DeepEqual(gotReports, tt.wantReports) {
				t.Errorf("gotReports = %v, want %v", gotReports, tt.wantReports)
			}

		})
	}
}