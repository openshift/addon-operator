package addon

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/metrics"
	"github.com/openshift/addon-operator/internal/ocm"
)

func (r *AddonReconciler) handleOCMAddOnStatusReporting(
	ctx context.Context,
	log logr.Logger,
	addon *addonsv1alpha1.Addon,
) (err error) {
	if !r.statusReportingRequired(addon) {
		log.Info("skipping status reporting")
		return nil
	}

	if r.ocmClient == nil {
		// OCM Client is not initialized.
		// Either the AddonOperatorReconciler did not yet create and inject the client or
		// the AddonOperator CR is not configured for OCM status reporting.
		//
		// All Addons will be requeued when the client becomes available for the first time.
		log.Info("delaying Addon status reporting to addon service endpoint until OCM client is initialized")

		return nil
	}

	log.Info("upserting addon status")
	err = r.postAddonStatus(ctx, addon)
	if err != nil {
		return err
	}

	// Before returning we store the current reported status
	// in the addon's status block.
	setLastReportedStatus(addon)
	return nil
}

func (r *AddonReconciler) postAddonStatus(ctx context.Context, addon *addonsv1alpha1.Addon) (err error) {
	statusPayload := ocm.AddOnStatusPostRequest{
		AddonID:          addon.Name,
		CorrelationID:    addon.Spec.CorrelationID,
		AddonVersion:     addon.Spec.Version,
		StatusConditions: mapToAddonStatusConditions(addon.Status.Conditions),
	}
	r.recordAddonServiceRequestDuration(func() {
		_, err = r.ocmClient.PostAddOnStatus(ctx, statusPayload)
	})
	return
}

func (r *AddonReconciler) statusReportingRequired(addon *addonsv1alpha1.Addon) bool {
	return r.statusReportingEnabled && isCurrentStatusDifferentFromPrevious(addon)
}

func (r *AddonReconciler) recordAddonServiceRequestDuration(reqFunc func()) {
	if metrics.IsMetricsRecorderInitialized() {
		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			us := v * 1000000 // convert to microseconds
			metrics.MetricsRecorder().RecordAddonServiceAPIRequests(us)
		}))
		defer timer.ObserveDuration()
	}
	reqFunc()
}

func mapToAddonStatusConditions(in []metav1.Condition) []addonsv1alpha1.AddOnStatusCondition {
	res := make([]addonsv1alpha1.AddOnStatusCondition, len(in))
	for i, obj := range in {
		res[i] = addonsv1alpha1.AddOnStatusCondition{
			StatusType:  obj.Type,
			StatusValue: obj.Status,
			Reason:      obj.Reason,
			Message:     obj.Message,
		}
	}
	return res
}

func isCurrentStatusDifferentFromPrevious(addon *addonsv1alpha1.Addon) bool {
	if addon.Status.OCMReportedStatusHash != nil {
		return addon.Status.OCMReportedStatusHash.StatusHash != HashCurrentAddonStatus(addon)
	}
	// If reported status is nil.
	return true
}

func setLastReportedStatus(addon *addonsv1alpha1.Addon) {
	addon.Status.OCMReportedStatusHash = &addonsv1alpha1.OCMAddOnStatusHash{
		StatusHash:         HashCurrentAddonStatus(addon),
		ObservedGeneration: addon.Generation,
	}
}
