package addon

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/go-logr/logr"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/ocm"
)

func (r *AddonReconciler) handleUpgradePolicyStatusReporting(
	ctx context.Context,
	log logr.Logger,
	addon *addonsv1alpha1.Addon,
) error {
	r.ocmClientMux.RLock()
	defer r.ocmClientMux.RUnlock()

	if r.ocmClient == nil {
		// OCM Client is not initialized.
		// Either the AddonOperatorReconciler did not yet create and inject the client or
		// the AddonOperator CR is not configured for OCM status reporting.
		//
		// All Addons will be requeued when the client becomes available for the first time.
		log.Info("delaying Addon status reporting to UpgradePolicy endpoint until OCM client is initialized")

		return nil
	}

	if addon.Spec.UpgradePolicy == nil || addon.UpgradeCompleteForCurrentVersion() {
		return nil
	}
	if addon.Status.UpgradePolicy == nil || addon.Status.UpgradePolicy.Version != addon.Spec.Version {
		return r.reportUpgradeStarted(ctx, addon)
	}
	if addon.IsAvailable() {
		return r.reportUpgradeCompleted(ctx, addon)
	}

	return nil
}

func (r *AddonReconciler) reportUpgradeStarted(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	var (
		policyID = addon.Spec.UpgradePolicy.ID
		version  = addon.Spec.Version
	)

	req := ocm.UpgradePolicyPatchRequest{
		ID:          policyID,
		Value:       ocm.UpgradePolicyValueStarted,
		Description: fmt.Sprintf("Upgrading addon to version %q.", version),
	}

	if err := r.handlePatchUpgradePolicy(ctx, req); err != nil {
		return fmt.Errorf(
			"patching UpgradePolicy %q at version %q as 'Started': %w", policyID, version, err,
		)
	}

	addon.SetUpgradePolicyStatus(addonsv1alpha1.AddonUpgradePolicyValueStarted)

	return nil
}

func (r *AddonReconciler) reportUpgradeCompleted(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	var (
		policyID = addon.Spec.UpgradePolicy.ID
		version  = addon.Spec.Version
	)

	req := ocm.UpgradePolicyPatchRequest{
		ID:          policyID,
		Value:       ocm.UpgradePolicyValueCompleted,
		Description: fmt.Sprintf("Addon was healthy at least once at version %q.", version),
	}

	if err := r.handlePatchUpgradePolicy(ctx, req); err != nil {
		return fmt.Errorf(
			"patching UpgradePolicy %q at version %q to 'Completed': %w", policyID, version, err,
		)
	}

	addon.SetUpgradePolicyStatus(addonsv1alpha1.AddonUpgradePolicyValueCompleted)

	return nil
}

// reconciler-facing wrapper around ocm.PatchUpgradePolicy that makes it
// easier to record OCM API metrics, and unit test the instrumentation.
// This also allows us to re-use the Recorder in AddonReconciler for recording
// OCM API metrics, rather than passing it down to the ocmClient object.
func (r *AddonReconciler) handlePatchUpgradePolicy(ctx context.Context,
	req ocm.UpgradePolicyPatchRequest) error {
	if r.Recorder != nil {
		// TODO: do not count metrics when API returns 5XX response
		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			us := v * 1000000 // convert to microseconds
			r.Recorder.RecordOCMAPIRequests(us)
		}))
		defer timer.ObserveDuration()
	}
	_, err := r.ocmClient.PatchUpgradePolicy(ctx, req)
	return err
}
