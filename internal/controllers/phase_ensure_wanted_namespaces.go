package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

// Ensure existence of Namespaces specified in the given Addon resource
// returns a bool that signals the caller to stop reconciliation and retry later
func (r *AddonReconciler) ensureWantedNamespaces(
	ctx context.Context, addon *addonsv1alpha1.Addon) (stopAndRetry bool, err error) {
	var unreadyNamespaces []string
	var collidedNamespaces []string

	for _, namespace := range addon.Spec.Namespaces {
		ensuredNamespace, err := r.ensureNamespace(ctx, addon, namespace.Name)
		if err != nil {
			if errors.Is(err, errNotOwnedByUs) {
				collidedNamespaces = append(collidedNamespaces, namespace.Name)
				continue
			}

			return false, err
		}

		if ensuredNamespace.Status.Phase != corev1.NamespaceActive {
			unreadyNamespaces = append(unreadyNamespaces, ensuredNamespace.Name)
		}
	}

	if len(collidedNamespaces) > 0 {
		msg := fmt.Sprintf("Namespaces with collisions: %s", strings.Join(collidedNamespaces, ", "))
		if err := r.reportPendingStatus(ctx, addon, addonsv1alpha1.AddonReasonCollidedNamespaces, msg); err != nil {
			return false, err
		}
		// collisions occured: signal caller to stop and retry
		return true, nil
	}

	if len(unreadyNamespaces) > 0 {
		msg := fmt.Sprintf("Namespaces not yet in Active phase: %s", strings.Join(unreadyNamespaces, ", "))
		return false, r.reportPendingStatus(ctx, addon, addonsv1alpha1.AddonReasonUnreadyNamespaces, msg)
	}

	return false, nil
}

// Ensure a single Namespace for the given Addon resource
func (r *AddonReconciler) ensureNamespace(ctx context.Context, addon *addonsv1alpha1.Addon, name string) (*corev1.Namespace, error) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{},
		},
	}
	addCommonLabels(namespace.Labels, addon)

	err := controllerutil.SetControllerReference(addon, namespace, r.Scheme)
	if err != nil {
		return nil, err
	}

	return reconcileNamespace(ctx, r.Client, namespace)
}

// reconciles a Namespace and returns the current object as observed.
// prevents adoption of Namespaces (unowned or owned by something else)
// reconciling a Namespace means: creating it when it is not present
// and erroring if our controller is not the owner of said Namespace
func reconcileNamespace(ctx context.Context, c client.Client, namespace *corev1.Namespace) (*corev1.Namespace, error) {

	currentNamespace := &corev1.Namespace{}

	{
		err := c.Get(ctx, client.ObjectKey{
			Name: namespace.Name,
		}, currentNamespace)
		if err != nil {
			if k8sApiErrors.IsNotFound(err) {
				return namespace, c.Create(ctx, namespace)
			}
			return nil, err
		}
	}

	if len(currentNamespace.OwnerReferences) == 0 ||
		!hasEqualControllerReference(currentNamespace, namespace) {
		return nil, errNotOwnedByUs
	}

	return currentNamespace, nil
}

// Tests if the controller reference on `wanted` matches the one on `current`
func hasEqualControllerReference(current, wanted metav1.Object) bool {
	currentOwnerRefs := current.GetOwnerReferences()

	var currentControllerRef *metav1.OwnerReference
	for _, ownerRef := range currentOwnerRefs {
		if *ownerRef.Controller {
			currentControllerRef = &ownerRef
			break
		}
	}

	if currentControllerRef == nil {
		return false
	}

	wantedOwnerRefs := wanted.GetOwnerReferences()

	for _, ownerRef := range wantedOwnerRefs {
		// OwnerRef is the same if UIDs match
		if currentControllerRef.UID == ownerRef.UID {
			return true
		}
	}

	return false
}
