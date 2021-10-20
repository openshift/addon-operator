package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	addonsPerPhaseTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "addon_operator_addons_phase_total",
			Help: "Total number of addons being reconciled for each phase: Pending, Ready, Termination, Error or Paused.",
		}, []string{"name", "phase"})

	addonsInstallationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "addon_operator_addons_installation_total",
			Help: "Total number of addons installations.",
		}, []string{"name"})

	// TODO - need to adjust the bucket here after we do some observation
	addonsInstallationSuccessTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "addon_operator_addons_installation_success_time_seconds",
			Help: "Tracking installation time per addon.",
			// minute wide buckets - 1 to 10 min install time
			Buckets: prometheus.LinearBuckets(60.0, 60.0, 10),
		}, []string{"name"})
)

func init() {
	// Register custom metrics with the controller runtime prom registry
	ctrlmetrics.Registry.MustRegister(
		addonsPerPhaseTotal,
		addonsInstallationTotal,
		addonsInstallationSuccessTime,
	)
}
