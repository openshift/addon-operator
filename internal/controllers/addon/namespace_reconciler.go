package addon

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
)

const NAMESPACE_RECONCILER_NAME = "namespaceReconciler"

type namespaceReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func (r *namespaceReconciler) Reconcile(ctx context.Context,
	addon *addonsv1alpha1.Addon) (reconcile.Result, error) {
	// Ensure wanted namespaces
	result, err := r.ensureWantedNamespaces(ctx, addon)
	if err != nil {
		err = errors.Join(err, controllers.ErrEnsureCreateNamespaces)
		return ctrl.Result{}, err
	} else if !result.IsZero() {
		return result, nil
	}

	// Ensure unwanted namespaces are removed
	if err := r.ensureDeletionOfUnwantedNamespaces(ctx, addon); err != nil {
		err = errors.Join(err, controllers.ErrEnsureDeleteNamespaces)
		return ctrl.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *namespaceReconciler) Name() string {
	return NAMESPACE_RECONCILER_NAME
}

// Ensure cleanup of Namespaces that are not needed anymore for the given Addon resource
func (r *namespaceReconciler) ensureDeletionOfUnwantedNamespaces(
	ctx context.Context, addon *addonsv1alpha1.Addon) error {
	currentNamespaces, err := getOwnedNamespacesViaCommonLabels(ctx, r.client, addon)
	if err != nil {
		return err
	}

	wantedNamespaceNames := make(map[string]struct{})
	for _, namespace := range addon.Spec.Namespaces {
		wantedNamespaceNames[namespace.Name] = struct{}{}
	}

	// Don't remove monitoring namespace as it will be handled
	// separately by `phase_delete_unwanted_monitoring_federation`
	wantedNamespaceNames[GetMonitoringNamespaceName(addon)] = struct{}{}

	for _, namespace := range currentNamespaces {
		_, isWanted := wantedNamespaceNames[namespace.Name]
		if isWanted {
			// don't delete
			continue
		}

		err := ensureNamespaceDeletion(ctx, r.client, namespace.Name)
		if err != nil {
			return err
		}
	}

	return nil
}

// Ensure that the given Namespace is deleted
func ensureNamespaceDeletion(ctx context.Context, c client.Client, name string) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := c.Delete(ctx, namespace)
	// don't propagate error if the Namespace is already gone
	if !k8sApiErrors.IsNotFound(err) {
		return err
	}
	return nil
}

// Get all Namespaces that have common labels matching the given Addon resource
func getOwnedNamespacesViaCommonLabels(
	ctx context.Context, c client.Client, addon *addonsv1alpha1.Addon) ([]corev1.Namespace, error) {
	selector := controllers.CommonLabelsAsLabelSelector(addon)

	list := &corev1.NamespaceList{}
	{
		err := c.List(ctx, list, &client.ListOptions{
			LabelSelector: client.MatchingLabelsSelector{
				Selector: selector,
			}})
		if err != nil {
			return nil, fmt.Errorf("could not list owned Namespaces: %w", err)
		}
	}

	return list.Items, nil
}

// Ensure existence of Namespaces specified in the given Addon resource
// returns a bool that signals the caller to stop reconciliation and retry later
func (r *namespaceReconciler) ensureWantedNamespaces(
	ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	var unreadyNamespaces []string

	for _, namespace := range addon.Spec.Namespaces {
		ensuredNamespace, err := r.ensureNamespace(ctx, addon, namespace.Name, WithNamespaceLabels(namespace.Labels), WithNamespaceAnnotations(namespace.Annotations))
		if err != nil {
			return ctrl.Result{}, err
		}

		if ensuredNamespace.Status.Phase != corev1.NamespaceActive {
			unreadyNamespaces = append(unreadyNamespaces, ensuredNamespace.Name)
		}
	}

	if len(unreadyNamespaces) > 0 {
		reportUnreadyNamespaces(addon, unreadyNamespaces)
		return ctrl.Result{RequeueAfter: defaultRetryAfterTime}, nil
	}

	return ctrl.Result{}, nil
}

// Ensure a single Namespace for the given Addon resource
func (r *namespaceReconciler) ensureNamespace(ctx context.Context, addon *addonsv1alpha1.Addon, name string, namespaceOpts ...NamespaceOpts) (*corev1.Namespace, error) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range namespaceOpts {
		opt := opt
		opt(namespace)
	}
	controllers.AddCommonLabels(namespace, addon)
	controllers.AddCommonAnnotations(namespace, addon)
	err := controllerutil.SetControllerReference(addon, namespace, r.scheme)
	if err != nil {
		return nil, err
	}
	return reconcileNamespace(ctx, r.client, namespace)
}

// reconciles a Namespace and returns the current object as observed.
// Warning: Will adopt existing Namespaces
// reconciling a Namespace means: creating it when it is not present
// and adopting it if our controller is not the owner of said Namespace
func reconcileNamespace(ctx context.Context, c client.Client, namespace *corev1.Namespace) (*corev1.Namespace, error) {
	currentNamespace := &corev1.Namespace{}

	if err := c.Get(ctx, client.ObjectKey{Name: namespace.Name}, currentNamespace); k8sApiErrors.IsNotFound(err) {
		return namespace, c.Create(ctx, namespace)
	} else if err != nil {
		return nil, err
	}

	currentNamespace.OwnerReferences = namespace.OwnerReferences

	currentLabels := labels.Set(currentNamespace.Labels)
	newLabels := labels.Merge(currentLabels, labels.Set(namespace.Labels))
	currentNamespace.Labels = newLabels

	currentAnnotations := labels.Set(currentNamespace.Annotations)
	newAnnotations := labels.Merge(currentAnnotations, labels.Set(namespace.Annotations))
	currentNamespace.Annotations = newAnnotations

	return currentNamespace, c.Update(ctx, currentNamespace)
}
