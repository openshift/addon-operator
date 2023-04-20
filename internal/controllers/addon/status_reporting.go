package addon

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
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

	// At this point, before returning we store the current reported status
	// in the addon's status block.
	defer func() {
		if err == nil {
			setLastReportedStatus(addon)
		}
	}()
	//We've added upsert functionality in the status API so modifying ADO to use POST
	log.Info("Making the Upsert call ")
	err = r.postAddonStatus(ctx, addon)
	if err != nil {

		return err
	}
	return nil
}

func (r *AddonReconciler) postAddonStatus(ctx context.Context, addon *addonsv1alpha1.Addon) (err error) {
	statusPayload := ocm.AddOnStatusPostRequest{
		AddonID:          addon.Name,
		CorrelationID:    addon.Spec.CorrelationID,
		StatusConditions: mapAddonStatusConditions(addon.Status.Conditions),
	}
	r.recordASRequestDuration(func() {
		_, err = r.ocmClient.PostAddOnStatus(ctx, statusPayload)
	})
	return
}

func (r *AddonReconciler) patchAddonStatus(ctx context.Context, addon *addonsv1alpha1.Addon) (err error) {
	payload := ocm.AddOnStatusPatchRequest{
		CorrelationID:    addon.Spec.CorrelationID,
		StatusConditions: mapAddonStatusConditions(addon.Status.Conditions),
	}
	r.recordASRequestDuration(func() {
		_, err = r.ocmClient.PatchAddOnStatus(ctx, addon.Name, payload)
	})
	return
}

func (r *AddonReconciler) statusReportingRequired(addon *addonsv1alpha1.Addon) bool {
	return r.statusReportingEnabled && currentStatusChangedFromPrevious(addon)
}

func (r *AddonReconciler) recordASRequestDuration(reqFunc func()) {
	if r.Recorder != nil {
		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			us := v * 1000000 // convert to microseconds
			r.Recorder.RecordAddonServiceAPIRequests(us)
		}))
		defer timer.ObserveDuration()
	}
	reqFunc()
}

func mapAddonStatusConditions(in []metav1.Condition) []addonsv1alpha1.AddOnStatusCondition {
	res := make([]addonsv1alpha1.AddOnStatusCondition, len(in))
	for i, obj := range in {
		res[i] = addonsv1alpha1.AddOnStatusCondition{
			StatusType:  obj.Type,
			StatusValue: obj.Status,
			Reason:      obj.Reason,
		}
	}
	return res
}

func currentStatusChangedFromPrevious(addon *addonsv1alpha1.Addon) bool {
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
