package controllers

import (
	"fmt"
	"io/ioutil"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

const (
	CommonManagedByLabel = "app.kubernetes.io/managed-by"
	CommonManagedByValue = "addon-operator"
	commonInstanceLabel  = "app.kubernetes.io/instance"
)

func AddCommonLabels(obj metav1.Object, addon *addonsv1alpha1.Addon) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	labels[CommonManagedByLabel] = CommonManagedByValue
	labels[commonInstanceLabel] = addon.Name
	obj.SetLabels(labels)
}

func CommonLabelsAsLabelSelector(addon *addonsv1alpha1.Addon) labels.Selector {
	labelSet := make(labels.Set)
	labelSet[CommonManagedByLabel] = CommonManagedByValue
	labelSet[commonInstanceLabel] = addon.Name
	return labelSet.AsSelector()
}

// Tests if the controller reference on `wanted` matches the one on `current`
func HasEqualControllerReference(current, wanted metav1.Object) bool {
	currentOwnerRefs := current.GetOwnerReferences()

	var currentControllerRef *metav1.OwnerReference
	for _, ownerRef := range currentOwnerRefs {
		or := ownerRef
		if *or.Controller {
			currentControllerRef = &or
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
	namespaceBytes, err := ioutil.ReadFile(inClusterNamespacePath)
	if err != nil {
		return "", fmt.Errorf("error reading namespace file: %w", err)
	}
	return string(namespaceBytes), nil
}
