package addon

import (
	"context"
	"fmt"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
)

// Ensures the presence or absence of an OperatorGroup depending on the Addon install type.
func (r *olmReconciler) ensureOperatorGroup(
	ctx context.Context, addon *addonsv1alpha1.Addon) (subReconcilerResult, error) {
	log := controllers.LoggerFromContext(ctx)
	commonConfig, stop := parseAddonInstallConfig(log, addon)
	if stop {
		return resultStop, nil
	}

	desiredOperatorGroup := &operatorsv1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controllers.DefaultOperatorGroupName,
			Namespace: commonConfig.Namespace,
			Labels:    map[string]string{},
		},
	}
	if addon.Spec.Install.Type == addonsv1alpha1.OLMOwnNamespace {
		desiredOperatorGroup.Spec.TargetNamespaces = []string{commonConfig.Namespace}
	}

	controllers.AddCommonLabels(desiredOperatorGroup, addon)
	controllers.AddCommonAnnotations(desiredOperatorGroup, addon)
	if err := controllerutil.SetControllerReference(addon, desiredOperatorGroup, r.scheme); err != nil {
		return resultNil, fmt.Errorf("setting controller reference: %w", err)
	}
	return resultNil, r.reconcileOperatorGroup(ctx, desiredOperatorGroup)
}

// Reconciles the Spec of the given OperatorGroup if needed by updating or creating the OperatorGroup.
// The given OperatorGroup is updated to reflect the latest state from the kube-apiserver.
func (r *olmReconciler) reconcileOperatorGroup(
	ctx context.Context, operatorGroup *operatorsv1.OperatorGroup) error {
	currentOperatorGroup := &operatorsv1.OperatorGroup{}

	err := r.client.Get(ctx, client.ObjectKeyFromObject(operatorGroup), currentOperatorGroup)
	if errors.IsNotFound(err) {
		return r.client.Create(ctx, operatorGroup)
	}
	if err != nil {
		return fmt.Errorf("getting OperatorGroup: %w", err)
	}

	currentLabels := labels.Set(currentOperatorGroup.Labels)
	newLabels := labels.Merge(currentLabels, labels.Set(operatorGroup.Labels))
	ownedByAddon := controllers.HasSameController(currentOperatorGroup, operatorGroup)
	specChanged := !equality.Semantic.DeepEqual(currentOperatorGroup.Spec, operatorGroup.Spec)
	if specChanged || !ownedByAddon || !labels.Equals(currentLabels, newLabels) {
		currentOperatorGroup.Spec = operatorGroup.Spec
		currentOperatorGroup.OwnerReferences = operatorGroup.OwnerReferences
		currentOperatorGroup.Labels = newLabels
		return r.client.Update(ctx, currentOperatorGroup)
	}
	return nil
}
