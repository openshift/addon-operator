package metrics

import (
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/meta"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

// addonState is a helper type that will help us
// track the conditions for an addon, in-memory.
// This state will be used for updating condition metrics.
type addonState struct {
	conditionMap map[string]addonConditions
	lock         sync.RWMutex
}

type addonConditions struct {
	available bool
	paused    bool
}

// Recorder stores all the metrics related to Addons.
type Recorder struct {
	addonState *addonState

	// metrics
	addonsCount           *prometheus.GaugeVec
	addonOperatorPaused   prometheus.Gauge // 0 - Not paused , 1 - Paused
	ocmAPIRequestDuration prometheus.Summary
	addonHealthInfo       *prometheus.GaugeVec
	// .. TODO: More metrics!
}

type addonCountLabel string

var (
	available addonCountLabel = "available"
	paused    addonCountLabel = "paused"
	total     addonCountLabel = "total"
)

func NewRecorder(register bool) *Recorder {

	addonsCount := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "addon_operator_addons_count",
			Help: "Total number of Addon installations, grouped by 'available', 'paused' and 'total'",
		}, []string{"count_by"})

	addonOperatorPaused := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "addon_operator_paused",
			Help: "A boolean that tells if the AddonOperator is paused",
		})

	ocmAPIReqDuration := prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "addon_operator_ocm_api_requests_durations",
			Help: "OCM API request latencies in microseconds",
			// p50, p90 and p99 latencies
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		})

	addonHealthInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "addon_operator_addon_health_info",
			Help: "Addon Health information",
		}, []string{"name", "version", "_id", "reason", "observed_generation"},
	)

	// Register metrics if `register` is true
	// This allows us to skip registering metrics
	// and re-use the recorder when testing.
	if register {
		ctrlmetrics.Registry.MustRegister(
			addonsCount,
			addonOperatorPaused,
			ocmAPIReqDuration,
			addonHealthInfo,
		)
	}

	return &Recorder{
		addonState: &addonState{
			conditionMap: map[string]addonConditions{},
		},
		addonsCount:           addonsCount,
		addonOperatorPaused:   addonOperatorPaused,
		ocmAPIRequestDuration: ocmAPIReqDuration,
		addonHealthInfo:       addonHealthInfo,
	}
}

func (r *Recorder) RecordAddonHealthInfo(addon *addonsv1alpha1.Addon, clusterID string) {

	var (
		// `healthStatus` defaults to unknown unless status conditions say otherwise
		healthStatus = 2
		healthReason = "Unknown"
		healthCond   = meta.FindStatusCondition(addon.Status.Conditions,
			addonsv1alpha1.Available)
	)

	if healthCond != nil {
		healthReason = healthCond.Reason

		switch healthCond.Status {
		case metav1.ConditionFalse:
			healthStatus = 0
		case metav1.ConditionTrue:
			healthStatus = 1
		default:
			healthStatus = 2
		}

	}

	addonVersion := "0.0.0" // default value when addon version is missing
	if addon.Status.ObservedVersion != "" {
		addonVersion = addon.Status.ObservedVersion
	}

	r.addonHealthInfo.WithLabelValues(addon.Name,
		addonVersion,
		clusterID,
		healthReason,
		fmt.Sprintf("%d", addon.Status.ObservedGeneration)).Set(float64(healthStatus))
}

// InjectOCMAPIRequestDuration allows us to override `r.ocmAPIRequestDuration` metric
// Useful while writing tests
func (r *Recorder) InjectOCMAPIRequestDuration(s prometheus.Summary) {
	r.ocmAPIRequestDuration = s
}

func (r *Recorder) increaseAvailableAddonsCount() {
	r.addonsCount.WithLabelValues(string(available)).Inc()
}

func (r *Recorder) decreaseAvailableAddonsCount() {
	r.addonsCount.WithLabelValues(string(available)).Dec()
}

func (r *Recorder) increasePausedAddonsCount() {
	r.addonsCount.WithLabelValues(string(paused)).Inc()
}

func (r *Recorder) decreasePausedAddonsCount() {
	r.addonsCount.WithLabelValues(string(paused)).Dec()
}

func (r *Recorder) increaseTotalAddonsCount() {
	r.addonsCount.WithLabelValues(string(total)).Inc()
}

func (r *Recorder) decreaseTotalAddonsCount() {
	r.addonsCount.WithLabelValues(string(total)).Dec()
}

func (r *Recorder) RecordOCMAPIRequests(us float64) {
	r.ocmAPIRequestDuration.Observe(us)
}

// SetAddonOperatorPaused sets the `addon_operator_paused` metric
// 0 - Not paused , 1 - Paused
func (r *Recorder) SetAddonOperatorPaused(paused bool) {
	if paused {
		r.addonOperatorPaused.Set(1)
	} else {
		r.addonOperatorPaused.Set(0)

	}
}

// RecordAddonMetrics is responsible for reconciling the following metrics:
// - addon_operator_addons_available
// - addon_operator_addons_paused
// - addon_operator_addons_total
func (r *Recorder) RecordAddonMetrics(addon *addonsv1alpha1.Addon) {
	r.addonState.lock.Lock()
	defer r.addonState.lock.Unlock()

	currCondition := addonConditions{
		available: meta.IsStatusConditionTrue(addon.Status.Conditions, addonsv1alpha1.Available),
		paused:    meta.IsStatusConditionTrue(addon.Status.Conditions, addonsv1alpha1.Paused),
	}

	addonUID := string(addon.UID)
	oldCondition, ok := r.addonState.conditionMap[addonUID]

	// handle new Addon installations
	if !ok {
		r.addonState.conditionMap[addonUID] = currCondition
		r.increaseTotalAddonsCount()
		if currCondition.available {
			r.increaseAvailableAddonsCount()
		}

		if currCondition.paused {
			r.increasePausedAddonsCount()
		}
		return
	}

	// reconcile metrics for existing Addons
	if oldCondition != currCondition {
		if oldCondition.available != currCondition.available {
			if currCondition.available {
				r.increaseAvailableAddonsCount()
			} else {
				r.decreaseAvailableAddonsCount()
			}
		}

		if oldCondition.paused != currCondition.paused {
			if currCondition.paused {
				r.increasePausedAddonsCount()
			} else {
				r.decreasePausedAddonsCount()
			}
		}

		// Update the current Addon conditions in the in-memory map
		r.addonState.conditionMap[addonUID] = currCondition
	}

	// handle Addon uninstallations
	if !addon.DeletionTimestamp.IsZero() {
		r.decreaseTotalAddonsCount()
		if currCondition.available {
			r.decreaseAvailableAddonsCount()
		}

		if currCondition.paused {
			r.decreasePausedAddonsCount()
		}
		delete(r.addonState.conditionMap, addonUID)
	}
}
