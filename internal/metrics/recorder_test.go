package metrics

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promTestUtil "github.com/prometheus/client_golang/prometheus/testutil"
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

// Tests ReconcileError
func TestReconcileError(t *testing.T) {
	expect_controller_name := "addon"
	expect_reconciler_error := "a top-level reconciler error"
	expect_subreconciler_error := "a sub reconciler error"
	var recorder *Recorder
	reconErr := NewReconcileError(
		expect_controller_name,
		recorder,
		false,
	)
	subReconErr := NewReconcileError(
		expect_controller_name,
		recorder,
		true,
	)

	// 1. Ensure it does not panic when no metrics recorder was created
	reconErr.Report(fmt.Errorf("an error"))

	recorder = NewRecorder(true, "clusterID")
	reconErr.SetRecorder(recorder)
	subReconErr.SetRecorder(recorder)

	errFromReconciler := fmt.Errorf(expect_reconciler_error)
	errFromSubReconciler := fmt.Errorf(expect_subreconciler_error)
	err := reconErr.Join(errFromReconciler, errFromSubReconciler)

	// 2. Ensure error from reconciler is processed correctly
	reconErr.Report(errFromReconciler)
	assert.Equal(t, errFromReconciler.Error(), reconErr.Reason())

	// 3. Ensure error from reconciler is processed correctly
	subReconErr.Report(err)
	assert.Equal(t, expect_subreconciler_error, subReconErr.Reason())

	// 4. Ensure metric is collected
	metric := recorder.GetReconcileErrorMetric()
	assert.NotNil(t, metric)
	// Ensure 2 values were collected due to the calls to Report
	assert.Equal(t, 2, promTestUtil.CollectAndCount(metric))
	// Ensure top-level reconciler error was collected
	controllerMetricVal := promTestUtil.ToFloat64(
		metric.WithLabelValues(
			expect_controller_name,
			expect_reconciler_error,
		),
	)
	assert.Equal(t, float64(1), controllerMetricVal)
	controllerMetricVal = promTestUtil.ToFloat64(
		metric.WithLabelValues(
			expect_controller_name,
			expect_subreconciler_error,
		),
	)
	assert.Equal(t, float64(1), controllerMetricVal)

	// 5. Ensure Join is returing as expected when values are nil
	var nilErr error
	err = reconErr.Join(nilErr, errFromSubReconciler)
	assert.Equal(t, nilErr, err)
	err = reconErr.Join(errFromReconciler, nilErr)
	assert.Equal(t, errFromReconciler, err)
}
