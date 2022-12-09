package addon

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
)

const SECRET_RECONCILER_NAME = "secretPropogationReconciler"

// Sub-Reconciler taking care of secret propagation.
type addonSecretPropagationReconciler struct {
	cachedClient, uncachedClient client.Client
	scheme                       *runtime.Scheme
	addonOperatorNamespace       string
}

func (r *addonSecretPropagationReconciler) Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	if addon.Spec.SecretPropagation == nil ||
		len(addon.Spec.SecretPropagation.Secrets) == 0 {
		// just ensure all propagated secrets are gone
		return ctrl.Result{}, r.cleanupUnknownSecrets(ctx, map[client.ObjectKey]struct{}{}, addon)
	}

	destinationSecretsWithoutNamespace, result, err := r.getDestinationSecretsWithoutNamespace(ctx, addon)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !result.IsZero() {
		return result, nil
	}

	knownSecrets, err := r.reconcileSecretsInAddonNamespaces(ctx, destinationSecretsWithoutNamespace, addon)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.cleanupUnknownSecrets(ctx, knownSecrets, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("propagated secret cleanup: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *addonSecretPropagationReconciler) Name() string {
	return SECRET_RECONCILER_NAME
}

// Lookup all secret sources for secret propagation
// returns a list of destination secrets, just missing their namespace
func (r *addonSecretPropagationReconciler) getDestinationSecretsWithoutNamespace(
	ctx context.Context,
	addon *addonsv1alpha1.Addon,
) ([]corev1.Secret, ctrl.Result, error) {
	var destinationSecrets []corev1.Secret
	if addon.Spec.SecretPropagation == nil {
		return nil, ctrl.Result{}, nil
	}

	for _, secretRef := range addon.Spec.SecretPropagation.Secrets {
		srcSecret, result, err := r.getReferencedSecret(ctx, addon, client.ObjectKey{
			Name:      secretRef.SourceSecret.Name,
			Namespace: r.addonOperatorNamespace,
		})
		if err != nil {
			return nil, ctrl.Result{}, err
		}
		if !result.IsZero() {
			return nil, result, nil
		}

		// Build destination secret -> will get applied into multiple addon namespaces
		destSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: secretRef.DestinationSecret.Name,
			},
			Data: srcSecret.Data,
			Type: srcSecret.Type,
		}
		controllers.AddCommonLabels(destSecret, addon)
		controllers.AddCommonAnnotations(destSecret, addon)
		if err := controllerutil.SetControllerReference(addon, destSecret, r.scheme); err != nil {
			return nil, ctrl.Result{}, fmt.Errorf("setting owner reference: %w", err)
		}
		destinationSecrets = append(destinationSecrets, *destSecret)
	}
	return destinationSecrets, ctrl.Result{}, nil
}

// Get a single referenced source secret for propagation
func (r *addonSecretPropagationReconciler) getReferencedSecret(
	ctx context.Context, addon *addonsv1alpha1.Addon, secretKey client.ObjectKey,
) (*corev1.Secret, ctrl.Result, error) {
	// Lookup configured secret.
	referencedSecret := &corev1.Secret{}

	err := r.cachedClient.Get(ctx, secretKey, referencedSecret)
	if errors.IsNotFound(err) {
		// the referenced secret might not be labeled correctly for the cache to pick up,
		// fallback to a uncached read to discover.
		if err := r.uncachedClient.Get(ctx, secretKey, referencedSecret); errors.IsNotFound(err) {
			// Secret does not exist for sure, break and keep retrying later.
			reportPendingStatus(addon, addonsv1alpha1.AddonReasonMissingSecretForPropagation, err.Error())
			return nil, ctrl.Result{RequeueAfter: defaultRetryAfterTime}, nil
		} else if err != nil {
			return nil, ctrl.Result{}, fmt.Errorf("getting source Secret for propagation via uncached client: %w", err)
		}

		// Update Secret to ensure it is part of our cache and we get events to reconcile.
		updatedReferenceSecret := referencedSecret.DeepCopy()
		if err := controllerutil.SetOwnerReference(addon, updatedReferenceSecret, r.scheme); err != nil {
			return nil, ctrl.Result{}, fmt.Errorf("adding OwnerReference for AddonOperator to referenced source Secret: %w", err)
		}
		if updatedReferenceSecret.Labels == nil {
			updatedReferenceSecret.Labels = map[string]string{}
		}
		updatedReferenceSecret.Labels[controllers.CommonCacheLabel] = controllers.CommonCacheValue
		if err := r.cachedClient.Patch(ctx, updatedReferenceSecret, client.MergeFrom(referencedSecret)); err != nil {
			return nil, ctrl.Result{}, fmt.Errorf("patching source Secret for cache and ownership: %w", err)
		}
	} else if err != nil {
		return referencedSecret, ctrl.Result{}, fmt.Errorf("getting source Secret for propagation: %w", err)
	}
	return referencedSecret, ctrl.Result{}, nil
}

// Reconcile secrets into all addon namespaces, returns a map of reconciled and thus known secret keys.
func (r *addonSecretPropagationReconciler) reconcileSecretsInAddonNamespaces(
	ctx context.Context, destinationSecretsWithoutNamespace []corev1.Secret,
	addon *addonsv1alpha1.Addon,
) (knownSecrets map[client.ObjectKey]struct{}, err error) {
	knownSecrets = map[client.ObjectKey]struct{}{}
	for _, destSecretWithoutNamespace := range destinationSecretsWithoutNamespace {
		for _, ns := range addon.Spec.Namespaces {
			destSecret := destSecretWithoutNamespace.DeepCopy()
			destSecret.Namespace = ns.Name
			key := client.ObjectKeyFromObject(destSecret)
			knownSecrets[key] = struct{}{}

			if err := reconcileSecret(ctx, r.cachedClient, destSecret); err != nil {
				return nil, fmt.Errorf("reconciling secret %s: %w", key, err)
			}
		}
	}
	return knownSecrets, nil
}

func (r *addonSecretPropagationReconciler) cleanupUnknownSecrets(
	ctx context.Context, knownSecrets map[client.ObjectKey]struct{},
	addon *addonsv1alpha1.Addon,
) error {
	secretList := &corev1.SecretList{}
	if err := r.cachedClient.List(ctx, secretList, client.MatchingLabelsSelector{
		Selector: controllers.CommonLabelsAsLabelSelector(addon),
	}); err != nil {
		return fmt.Errorf("listing secrets for delete check: %w", err)
	}
	for i := range secretList.Items {
		secret := &secretList.Items[i]
		key := client.ObjectKeyFromObject(secret)
		if _, ok := knownSecrets[key]; ok {
			// secret is known to us and should continue to exist
			continue
		}

		if err := r.cachedClient.Delete(ctx, secret); err != nil {
			return fmt.Errorf("deleting unknown propagated secret: %w", err)
		}
	}
	return nil
}

func reconcileSecret(
	ctx context.Context, c client.Client, desiredSecret *corev1.Secret) error {
	actualSecret := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKeyFromObject(desiredSecret), actualSecret)
	if errors.IsNotFound(err) {
		if err := c.Create(ctx, desiredSecret); err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("creating secret: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("getting secret: %w", err)
	}

	if actualSecret.Labels == nil {
		actualSecret.Labels = map[string]string{}
	}
	for k, v := range desiredSecret.GetLabels() {
		actualSecret.Labels[k] = v
	}
	// Type is immutable, so we can't reconcile it
	// actualSecret.Type = desiredSecret.Type
	actualSecret.Data = desiredSecret.Data
	actualSecret.OwnerReferences = desiredSecret.OwnerReferences

	if err := c.Update(ctx, actualSecret); err != nil {
		return fmt.Errorf("updating secret: %w", err)
	}
	return nil
}
