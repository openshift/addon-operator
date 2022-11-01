package addon

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

const (
	CSVGroup   string = "operators.coreos.com"
	CSVVersion string = "v1alpha1"
	CSVKind    string = "ClusterServiceVersion"
)

func (r *olmReconciler) observeCurrentCSV(
	ctx context.Context,
	addon *addonsv1alpha1.Addon,
	csvKey client.ObjectKey,
) (requeueResult, error) {
	phase, err := r.getCSVPhase(ctx, csvKey)
	if err != nil {
		return resultNil, fmt.Errorf("finding installed CSV phase: %w", err)
	}

	var message string
	switch phase {
	case operatorsv1alpha1.CSVPhaseSucceeded:
		// do nothing here
	case operatorsv1alpha1.CSVPhaseFailed:
		message = "failed"
	default:
		message = "unkown/pending"
	}

	if message != "" {
		reportUnreadyCSV(addon, message)
		return resultRetry, nil
	}
	return resultNil, nil
}

func (r *olmReconciler) getCSVPhase(
	ctx context.Context,
	csvKey client.ObjectKey,
) (operatorsv1alpha1.ClusterServiceVersionPhase, error) {
	csv := &unstructured.Unstructured{}
	gvk := schema.GroupVersionKind{
		Group:   CSVGroup,
		Version: CSVVersion,
		Kind:    CSVKind,
	}
	csv.SetGroupVersionKind(gvk)
	if err := r.client.Get(ctx, csvKey, csv); err != nil {
		return "", fmt.Errorf("getting CSV: %w", err)
	}
	phase, ok, err := unstructured.NestedString(csv.Object, "status", "phase")
	if err != nil {
		return "", fmt.Errorf("getting csv.status.phase: %w", err)
	}
	if !ok {
		return "", nil
	}
	return operatorsv1alpha1.ClusterServiceVersionPhase(phase), nil
}
