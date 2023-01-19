package controllers

import (
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

const (
	CommonManagedByLabel = "app.kubernetes.io/managed-by"
	CommonManagedByValue = "addon-operator"
	CommonCacheLabel     = "addons.managed.openshift.io/cached"
	CommonCacheValue     = "addon-operator"
	CommonInstanceLabel  = "app.kubernetes.io/instance"
)

const (
	MSOLabel = "addons.managed.openshift.io/mso"
)

func AddCommonLabels(obj metav1.Object, addon *addonsv1alpha1.Addon) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	labels[CommonManagedByLabel] = CommonManagedByValue
	labels[CommonCacheLabel] = CommonCacheValue
	labels[CommonInstanceLabel] = addon.Name
	if len(addon.Spec.CommonLabels) != 0 {
		labels = k8slabels.Merge(labels, addon.Spec.CommonLabels)
	}
	obj.SetLabels(labels)
}

func AddCommonAnnotations(obj metav1.Object, addon *addonsv1alpha1.Addon) {
	if len(addon.Spec.CommonAnnotations) == 0 {
		return
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations = k8slabels.Merge(annotations, addon.Spec.CommonAnnotations)
	obj.SetAnnotations(annotations)
}

func CommonLabelsAsLabelSelector(addon *addonsv1alpha1.Addon) labels.Selector {
	labelSet := make(labels.Set)
	labelSet[CommonManagedByLabel] = CommonManagedByValue
	labelSet[CommonCacheLabel] = CommonCacheValue
	labelSet[CommonInstanceLabel] = addon.Name
	return labelSet.AsSelector()
}

// Tests if two objects have the same controller
func HasSameController(objA, objB metav1.Object) bool {
	controllerA := metav1.GetControllerOf(objA)
	controllerB := metav1.GetControllerOf(objB)
	if controllerA == nil || controllerB == nil {
		return false
	}
	return controllerA.UID == controllerB.UID
}

const inClusterNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// Returns the namespace we are currently running in.
func CurrentNamespace() (namespace string, err error) {
	// allow for local override, when we don't run in-cluster.
	if ns, ok := os.LookupEnv("ADDON_OPERATOR_NAMESPACE"); ok && len(ns) != 0 {
		return ns, nil
	}

	// Check whether the namespace file exists.
	// If not, we are not running in cluster so can't guess the namespace.
	if _, err := os.Stat(inClusterNamespacePath); os.IsNotExist(err) {
		return "", fmt.Errorf("not running in-cluster, please specify ADDON_OPERATOR_NAMESPACE")
	} else if err != nil {
		return "", fmt.Errorf("error checking namespace file: %w", err)
	}

	// Load the namespace file and return its content
	namespaceBytes, err := os.ReadFile(inClusterNamespacePath)
	if err != nil {
		return "", fmt.Errorf("error reading namespace file: %w", err)
	}
	return string(namespaceBytes), nil
}
