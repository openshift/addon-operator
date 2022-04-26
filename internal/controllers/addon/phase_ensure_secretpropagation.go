package addon

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
)

// propagates secrets into addon namespaces
func (r *AddonReconciler) ensureSecretPropagation(
	ctx context.Context, log logr.Logger,
	addon *addonsv1alpha1.Addon,
) (requeueResult, error) {
	var secrets []addonsv1alpha1.AddonSecretPropagationReference
	if addon.Spec.SecretPropagation != nil {
		secrets = addon.Spec.SecretPropagation.Secrets
	}

	// Lookup source secrets
	var destinationSecrets []corev1.Secret
	for _, secretRef := range secrets {
		srcSecret, result, err := getReferencedSecret(ctx, log, r.Client, r.UncachedClient, r.Scheme, addon, client.ObjectKey{
			Name:      secretRef.SourceSecret.Name,
			Namespace: r.AddonOperatorNamespace,
		})
		if err != nil {
			return resultNil, err
		}
		if result != resultNil {
			return result, nil
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
		if err := controllerutil.SetControllerReference(addon, destSecret, r.Scheme); err != nil {
			return resultNil, fmt.Errorf("setting owner reference: %w", err)
		}
		destinationSecrets = append(destinationSecrets, *destSecret)
	}

	// Distribute secrets to addon namespaces
	knownSecrets := map[client.ObjectKey]struct{}{}
	for _, destSecretTemplate := range destinationSecrets {
		for _, ns := range addon.Spec.Namespaces {
			destSecret := destSecretTemplate.DeepCopy()
			destSecret.Namespace = ns.Name
			key := client.ObjectKeyFromObject(destSecret)
			knownSecrets[key] = struct{}{}

			if err := reconcileSecret(ctx, r.Client, destSecret); err != nil {
				return resultNil, fmt.Errorf("reconciling secret %s: %w", key, err)
			}
		}
	}

	// Delete all other propagated secrets that might be left over
	secretList := &corev1.SecretList{}
	if err := r.Client.List(ctx, secretList); err != nil {
		return resultNil, fmt.Errorf("listing secrets for delete check: %w", err)
	}
	for i := range secretList.Items {
		secret := &secretList.Items[i]
		key := client.ObjectKeyFromObject(secret)
		if _, ok := knownSecrets[key]; ok {
			// secret is known to us and should continue to exist
			continue
		}

		if err := r.Client.Delete(ctx, secret); err != nil {
			return resultNil, fmt.Errorf("deleting unknown propagated secret: %w", err)
		}
	}

	return resultNil, nil
}

func getReferencedSecret(
	ctx context.Context,
	log logr.Logger,
	c client.Client, uncachedClient client.Client,
	scheme *runtime.Scheme,
	addon *addonsv1alpha1.Addon,
	pullSecretKey client.ObjectKey,
) (*corev1.Secret, requeueResult, error) {
	// Lookup configured secret.
	referencedSecret := &corev1.Secret{}

	err := c.Get(ctx, pullSecretKey, referencedSecret)
	if errors.IsNotFound(err) {
		// the referenced secret might not be labeled correctly for the cache to pick up,
		// fallback to a uncached read to discover.
		if err := uncachedClient.Get(ctx, pullSecretKey, referencedSecret); errors.IsNotFound(err) {
			// Secret does not exist for sure, break and keep retrying later.
			reportPendingStatus(addon, addonsv1alpha1.AddonReasonMissingSecretForPropagation, err.Error())
			return nil, resultRetry, nil
		} else if err != nil {
			return nil, resultNil, fmt.Errorf("getting source Secret for propagation via uncached client: %w", err)
		}
		log.Info(
			fmt.Sprintf(
				"found addon pull secret via uncached read, ensuring the referenced secret is labeled with %s=%s",
				controllers.CommonManagedByLabel, controllers.CommonManagedByValue),
		)

		// Update Secret to ensure it is part of our cache and we get events to reconcile.
		updatedReferenceSecret := referencedSecret.DeepCopy()
		if err := controllerutil.SetOwnerReference(addon, updatedReferenceSecret, scheme); err != nil {
			return nil, resultNil, fmt.Errorf("adding OwnerReference for AddonOperator to referenced source Secret: %w", err)
		}
		controllers.AddCommonLabels(updatedReferenceSecret, addon)
		if err := c.Patch(ctx, updatedReferenceSecret, client.MergeFrom(referencedSecret)); err != nil {
			return nil, resultNil, fmt.Errorf("patching source Secret for cache and ownership: %w", err)
		}
	} else if err != nil {
		return referencedSecret, resultNil, fmt.Errorf("getting source Secret for propagation: %w", err)
	}
	return referencedSecret, resultNil, nil
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
