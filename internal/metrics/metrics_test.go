package metrics

import (
	"fmt"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

func newTestAddon(uid string, conditions []metav1.Condition) *addonsv1alpha1.Addon {
	return &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID(uid),
		},
		Status: addonsv1alpha1.AddonStatus{
			Conditions: conditions,
		},
	}
}

func TestAddonMetrics_InstallCount(t *testing.T) {
	recorder := NewRecorder(false, "sdjkl83hjd")

	addons := []*addonsv1alpha1.Addon{
		newTestAddon("o672wxBaW9iR", []metav1.Condition{}),
		newTestAddon("kpzLavSo27F8", []metav1.Condition{}),
	}

	t.Run("no addons installed", func(t *testing.T) {
		// Expected:
		// addon_operator_addons_count{count_by="total"} 0
		assert.Equal(t, float64(0), testutil.ToFloat64(
			recorder.addonsCount.WithLabelValues(string(available))))
	})

	t.Run("new addon(s) installed", func(t *testing.T) {
		recorder.RecordAddonMetrics(addons[0])

		// Expected:
		// addon_operator_addons_count{count_by="total"} 1
		assert.Equal(t, float64(1), testutil.ToFloat64(
			recorder.addonsCount.WithLabelValues(string(total))))

		recorder.RecordAddonMetrics(addons[1])

		// Expected:
		// addon_operator_addons_count{count_by="total"} 2
		assert.Equal(t, float64(2), testutil.ToFloat64(
			recorder.addonsCount.WithLabelValues(string(total))))
	})

	t.Run("addon(s) uninstalled", func(t *testing.T) {
		now := metav1.NewTime(time.Now())
		addons[0].DeletionTimestamp = &now
		recorder.RecordAddonMetrics(addons[0])

		// Expected:
		// addon_operator_addons_count{count_by="total"} 1
		assert.Equal(t, float64(1), testutil.ToFloat64(
			recorder.addonsCount.WithLabelValues(string(total))))

		addons[1].DeletionTimestamp = &now
		recorder.RecordAddonMetrics(addons[1])

		// Expected:
		// addon_operator_addons_count{count_by="total"} 0
		assert.Equal(t, float64(0), testutil.ToFloat64(
			recorder.addonsCount.WithLabelValues(string(total))))
	})
}

func TestAddonMetrics_AddonHealth(t *testing.T) {
	var (
		recorder        = NewRecorder(false, "fdj41ddk")
		addonNamePrefix = "test-addon-"
	)

	// synthetic test cases for addon health
	testCases := []struct {
		addon    *addonsv1alpha1.Addon
		expected float64
	}{
		{
			addon: newTestAddon("o672wxBaW9iR", []metav1.Condition{
				{
					Type:   addonsv1alpha1.Available,
					Status: metav1.ConditionFalse,
					Reason: addonsv1alpha1.AddonReasonUnreadyCSV,
				},
			}),
			expected: float64(0),
		},
		{
			addon: newTestAddon("o672wxBaW9iR", []metav1.Condition{
				{
					Type:   addonsv1alpha1.Available,
					Status: metav1.ConditionTrue,
					Reason: addonsv1alpha1.AddonReasonFullyReconciled,
				},
			}),
			expected: float64(1),
		},
		{
			addon: newTestAddon("o672wxBaW9iR", []metav1.Condition{
				{
					Type:   addonsv1alpha1.Available,
					Status: metav1.ConditionUnknown,
				},
			}),
			expected: float64(2),
		},
		{
			addon:    newTestAddon("o672wxBaW9iR", []metav1.Condition{}),
			expected: float64(2),
		},
	}

	// iterating over all the test cases
	for i, tc := range testCases {
		t.Run("addon health info metric test", func(t *testing.T) {
			// local copy of the addon
			addon := tc.addon.DeepCopy()
			addon.Name = fmt.Sprintf("%s-%d", addonNamePrefix, i)
			addonConditions := addon.Status.Conditions

			healthReason := "Unknown"

			if len(addonConditions) != 0 {
				healthReason = addonConditions[0].Reason
			}

			recorder.recordAddonHealthInfo(addon)
			assert.Equal(t, float64(tc.expected), testutil.ToFloat64(
				recorder.addonHealthInfo.WithLabelValues(
					addon.Name,
					"0.0.0",
					healthReason,
				),
			))
		})
	}
}

func TestAddonMetrics_AddonConditions(t *testing.T) {
	recorder := NewRecorder(false, "asdf1234")
	addon := newTestAddon("o672wxBaW9iR", []metav1.Condition{})

	t.Run("uninitialized conditions", func(t *testing.T) {
		recorder.RecordAddonMetrics(addon)

		// Expected:
		// addon_operator_addons_count{count_by="paused"} 0
		// addon_operator_addons_count{count_by="available"} 0
		assert.Equal(t, float64(0), testutil.ToFloat64(
			recorder.addonsCount.WithLabelValues(string(paused))))
		assert.Equal(t, float64(0), testutil.ToFloat64(
			recorder.addonsCount.WithLabelValues(string(available))))
	})

	// create a matrix of different combinations for available and paused
	for _, isAvailable := range []metav1.ConditionStatus{metav1.ConditionTrue, metav1.ConditionFalse} {
		for _, isPaused := range []metav1.ConditionStatus{metav1.ConditionTrue, metav1.ConditionFalse} {
			t.Run(fmt.Sprintf("addon available: %v, addon paused: %v", available, paused), func(t *testing.T) {

				// create local copy within this closure
				addon := addon.DeepCopy()

				expectedAvailable := 0
				if isAvailable == metav1.ConditionTrue {
					expectedAvailable = 1
				}
				expectedPaused := 0
				if isPaused == metav1.ConditionTrue {
					expectedPaused = 1
				}

				conditions := []metav1.Condition{
					{
						Type:   addonsv1alpha1.Available,
						Status: isAvailable,
					},
					{
						Type:   addonsv1alpha1.Paused,
						Status: isPaused,
					},
				}
				addon.Status.Conditions = conditions
				recorder.RecordAddonMetrics(addon)

				assert.Equal(t, float64(expectedPaused),
					testutil.ToFloat64(recorder.addonsCount.WithLabelValues(string(paused))))
				assert.Equal(t, float64(expectedAvailable),
					testutil.ToFloat64(recorder.addonsCount.WithLabelValues(string(available))))

			})
		}
	}

	t.Run("addon operator paused", func(t *testing.T) {
		recorder.SetAddonOperatorPaused(true)

		// Expected:
		// addon_operator_paused{} 1
		assert.Equal(t, float64(1), testutil.ToFloat64(recorder.addonOperatorPaused))
	})

	t.Run("addon operator unpaused", func(t *testing.T) {
		recorder.SetAddonOperatorPaused(false)

		// Expected:
		// addon_operator_paused{} 0
		assert.Equal(t, float64(0), testutil.ToFloat64(recorder.addonOperatorPaused))
	})
}

// TestInjectOCMAPIRequestDuration verifies the behavior of the InjectOCMAPIRequestDuration
// method. It ensures the provided summary object is assigned to the ocmAPIRequestDuration field.
func TestInjectOCMAPIRequestDuration(t *testing.T) {
	r := &Recorder{}
	summary := prometheus.NewSummary(prometheus.SummaryOpts{Name: "test_summary"})
	r.InjectOCMAPIRequestDuration(summary)

	// Verify that the injected summary is correctly assigned
	assert.Equal(t, summary, r.ocmAPIRequestDuration, "Expected injected OCM API request duration summary to be %v", summary)
}

// TestInjectAddonServiceAPIRequestDuration verifies the behavior of the InjectAddonServiceAPIRequestDuration
// method. It ensures that the provided summary object is assigned to the addonServiceAPIRequestDuration field.
func TestInjectAddonServiceAPIRequestDuration(t *testing.T) {
	r := &Recorder{}
	summary := prometheus.NewSummary(prometheus.SummaryOpts{Name: "test_summary"})
	r.InjectAddonServiceAPIRequestDuration(summary)

	// Verify that the injected summary is correctly assigned
	assert.Equal(t, summary, r.addonServiceAPIRequestDuration, "Expected injected addon service API request duration summary to be %v", summary)
}
