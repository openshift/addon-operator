package metrics

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promTestUtil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	controllers "github.com/openshift/addon-operator/controllers"
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
	expect_addon_name := "reference-addon"

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
	reconErr.Report(controllers.ErrGetAddon, expect_addon_name)

	recorder = NewRecorder(true, "clusterID")
	reconErr.SetRecorder(recorder)
	subReconErr.SetRecorder(recorder)

	err := reconErr.Join(fmt.Errorf("an arbitrary error"), controllers.ErrUpdateAddon)

	// 2. Ensure error from reconciler is processed correctly
	reconErr.Report(controllers.ErrGetAddon, expect_addon_name)
	assert.Equal(t, controllers.ErrGetAddon.Error(), reconErr.Reason())

	// 3. Ensure error from subreconciler is processed correctly
	subReconErr.Report(err, expect_addon_name)
	assert.Equal(t, controllers.ErrUpdateAddon.Error(), subReconErr.Reason())

	// 4. Ensure metric is collected
	metric := recorder.GetReconcileErrorMetric()
	assert.NotNil(t, metric)
	// Ensure 2 values were collected due to the calls to Report
	assert.Equal(t, 2, promTestUtil.CollectAndCount(metric))
	// Ensure top-level reconciler error was collected
	controllerMetricVal := promTestUtil.ToFloat64(
		metric.WithLabelValues(
			expect_controller_name,
			controllers.ErrGetAddon.Error(),
			expect_addon_name,
		),
	)
	assert.Equal(t, float64(1), controllerMetricVal)
	controllerMetricVal = promTestUtil.ToFloat64(
		metric.WithLabelValues(
			expect_controller_name,
			controllers.ErrUpdateAddon.Error(),
			expect_addon_name,
		),
	)
	assert.Equal(t, float64(1), controllerMetricVal)

	//5. Ensure Join is returning as expected when values are nil
	var nilErr error
	err = reconErr.Join(nilErr, controllers.ErrGetAddon)
	assert.Equal(t, controllers.ErrGetAddon, err)
	err = reconErr.Join(controllers.ErrGetAddon, nilErr)
	assert.Equal(t, controllers.ErrGetAddon, err)
	err = reconErr.Join(nil, nil)
	assert.Equal(t, nilErr, err)

	// 6. Ensure it only reports errors of type "ControllerReconcileError"
	unclassifiedErr := fmt.Errorf("this is an unclassified error")
	expectReason := subReconErr.Reason()
	subReconErr.Report(unclassifiedErr, expect_addon_name)
	assert.Equal(t, expectReason, subReconErr.Reason())
	classifiedErr := controllers.ErrGetAddon
	subReconErr.Report(classifiedErr, expect_addon_name)
	assert.Equal(t, subReconErr.Reason(), controllers.ErrGetAddon.Error())
}
