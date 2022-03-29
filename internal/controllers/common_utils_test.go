package controllers

import (
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestHasEqualControllerReference(t *testing.T) {
	require.True(t, HasEqualControllerReference(
		testutil.NewTestNamespace(),
		testutil.NewTestNamespace(),
	))

	require.False(t, HasEqualControllerReference(
		testutil.NewTestNamespace(),
		testutil.NewTestExistingNamespace(),
	))

	require.False(t, HasEqualControllerReference(
		testutil.NewTestNamespace(),
		testutil.NewTestExistingNamespaceWithoutOwner(),
	))
}

func TestAddCommonLabels(t *testing.T) {
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: v1.ObjectMeta{
			Name: "test",
		},
	}

	obj := &unstructured.Unstructured{} // some arbitrary object
	AddCommonLabels(obj, addon)

	labels := obj.GetLabels()
	if labels[commonInstanceLabel] != addon.Name {
		t.Error("commonInstanceLabel was not set to addon name")
	}

	if labels[commonManagedByLabel] != commonManagedByValue {
		t.Error("commonManagedByLabel was not set to operator name")
	}
}

func TestCommonLabelsAsLabelSelector(t *testing.T) {
	addonWithCorrectName := &addonsv1alpha1.Addon{
		ObjectMeta: v1.ObjectMeta{
			Name: "test",
		},
	}
	selector := CommonLabelsAsLabelSelector(addonWithCorrectName)

	if selector.Empty() {
		t.Fatal("selector is empty but should filter on common labels")
	}
}
