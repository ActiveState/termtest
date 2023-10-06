package termtest

import (
	"errors"
	"reflect"
	"sync"
	"testing"

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
				return newOutputConsumer(func(buffer string) (endPos int, err error) {
					*reports = append(*reports, buffer)
					return 1, nil
				}, OptConsInherit(newTestOpts(nil, t)))
			},
			[]string{"Report"},
			[]string{"Report"},
			nil,
			nil,
		},
		{
			"Multiple reports",
			func(reports *[]string) *outputConsumer {
				return newOutputConsumer(func(buffer string) (endPos int, err error) {
					*reports = append(*reports, buffer)
					return boolToInt(buffer != "Three"), nil
				}, OptConsInherit(newTestOpts(nil, t)))
			},
			[]string{"One", "Two", "Three"},
			[]string{"One", "Two", "Three"},
			nil,
			nil,
		},
		{
			"Consumer error",
			func(reports *[]string) *outputConsumer {
				return newOutputConsumer(func(buffer string) (endPos int, err error) {
					return 1, testConsumerError
				}, OptConsInherit(newTestOpts(nil, t)))
			},
			[]string{"Report"},
			[]string{},
			testConsumerError,
			testConsumerError,
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
					_, err := oc.Report([]byte(report))
					assert.ErrorIs(t, err, tt.wantReportErr)
				}
				wg.Done()
			}()

			err := oc.wait()
			assert.ErrorIs(t, err, tt.wantWaitErr)

			wg.Wait()

			if !reflect.DeepEqual(gotReports, tt.wantReports) {
				t.Errorf("gotReports = %v, want %v", gotReports, tt.wantReports)
			}

		})
	}
}
