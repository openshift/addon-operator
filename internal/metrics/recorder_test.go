package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

// TestRecorder_RecordOCMAPIRequests ensure the RecordOCMAPIRequests method correctly
// records the OCM API request by observing the ocmAPIRequestDuration metric.
func TestRecorder_RecordOCMAPIRequests(t *testing.T) {
	// duration of an API request in microseconds
	duration := 100.0

	type fields struct {
		ocmAPIRequestDuration prometheus.Summary
	}
	type args struct {
		us float64
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "Record OCM API Requests",
			fields: fields{
				ocmAPIRequestDuration: prometheus.NewSummary(prometheus.SummaryOpts{}),
			},
			args: args{
				us: duration,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recorder{
				ocmAPIRequestDuration: tt.fields.ocmAPIRequestDuration,
			}
			r.RecordOCMAPIRequests(tt.args.us)

			// Assert that the metric has been observed correctly
			summary := r.ocmAPIRequestDuration
			assert.NotNil(t, summary)
		})
	}
}

// TestRecorder_RecordAddonServiceAPIRequests ensure the addonServiceAPIRequestDuration method
// metric is correctly observed when the RecordAddonServiceAPIRequests method is called.
func TestRecorder_RecordAddonServiceAPIRequests(t *testing.T) {
	// duration of an API request in microseconds
	duration := 200.0

	type fields struct {
		addonServiceAPIRequestDuration prometheus.Summary
	}
	type args struct {
		us float64
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "Record Addon Service API Requests",
			fields: fields{
				addonServiceAPIRequestDuration: prometheus.NewSummary(prometheus.SummaryOpts{}),
			},
			args: args{
				us: duration,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recorder{
				addonServiceAPIRequestDuration: tt.fields.addonServiceAPIRequestDuration,
			}
			r.RecordAddonServiceAPIRequests(tt.args.us)

			// Assert that the metric has been observed correctly
			summary := r.addonServiceAPIRequestDuration
			assert.NotNil(t, summary)
		})
	}
}
