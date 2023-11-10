package controllers

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestHasEqualControllerReference(t *testing.T) {
	require.True(t, HasSameController(
		testutil.NewTestNamespace(),
		testutil.NewTestNamespace(),
	))

	require.False(t, HasSameController(
		testutil.NewTestNamespace(),
		testutil.NewTestExistingNamespace(),
	))

	require.False(t, HasSameController(
		testutil.NewTestNamespace(),
		testutil.NewTestNamespaceWithoutOwner(),
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
	if labels[CommonInstanceLabel] != addon.Name {
		t.Error("commonInstanceLabel was not set to addon name")
	}

	if labels[CommonManagedByLabel] != CommonManagedByValue {
		t.Error("commonManagedByLabel was not set to operator name")
	}

	if labels[CommonCacheLabel] != CommonCacheValue {
		t.Error("commonCacheLabel was not set to operator name")
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

// The TestAddCommonAnnotations function tests adding common annotations from an Addon
// object to a metav1.Object object
func TestAddCommonAnnotations(t *testing.T) {
	type args struct {
		obj   metav1.Object
		addon *addonsv1alpha1.Addon
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "EmptyObject",
			args: args{
				obj:   &metav1.ObjectMeta{},
				addon: &addonsv1alpha1.Addon{},
			},
		},
		{
			name: "ObjectWithExistingAnnotations",
			args: args{
				obj: &metav1.ObjectMeta{
					Annotations: map[string]string{
						"existing.annotation": "existingValue",
					},
				},
				addon: &addonsv1alpha1.Addon{},
			},
		},
		{
			name: "ObjectWithCommonAnnotations",
			args: args{
				obj: &metav1.ObjectMeta{},
				addon: &addonsv1alpha1.Addon{
					Spec: addonsv1alpha1.AddonSpec{
						CommonAnnotations: map[string]string{
							"test.annotation1": "lpsre",
							"test.annotation2": "mtsre",
						},
					},
				},
			},
		},
		{
			name: "ObjectWithExistingAndCommonAnnotations",
			args: args{
				obj: &metav1.ObjectMeta{
					Annotations: map[string]string{
						"existing.annotation": "existingValue",
					},
				},
				addon: &addonsv1alpha1.Addon{
					Spec: addonsv1alpha1.AddonSpec{
						CommonAnnotations: map[string]string{
							"test.annotation1": "lpsre",
							"test.annotation2": "mtsre",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Copy the original annotations to compare them later
			originalAnnotations := make(map[string]string)
			for k, v := range tt.args.obj.GetAnnotations() {
				originalAnnotations[k] = v
			}

			AddCommonAnnotations(tt.args.obj, tt.args.addon)

			// Check if the common annotations have been added
			for k, v := range tt.args.addon.Spec.CommonAnnotations {
				assert.Equal(t, v, tt.args.obj.GetAnnotations()[k], "Expected common annotation %s to have value %s", k, v)
			}

			// Check if the original annotations are still present
			for k, v := range originalAnnotations {
				assert.Equal(t, v, tt.args.obj.GetAnnotations()[k], "Expected original annotation %s to have value %s", k, v)
			}

			// Check if any additional annotations were added apart from the common ones
			for k, v := range tt.args.obj.GetAnnotations() {
				_, commonAnnotationExists := tt.args.addon.Spec.CommonAnnotations[k]
				_, originalAnnotationExists := originalAnnotations[k]
				require.Truef(t, commonAnnotationExists || originalAnnotationExists, "Unexpected annotation %s with value %s has been added", k, v)
			}
		})
	}
}

// TestCurrentNamespace tests the CurrentNamespace function to ensure it
// behaves correctly under different scenarios.
func TestCurrentNamespace(t *testing.T) {

	tests := []struct {
		name          string
		wantNamespace string
		wantErr       bool
	}{
		{
			name:          "Running in-cluster",
			wantNamespace: "test-namespace",
			wantErr:       false,
		},
		{
			name:          "Running outside cluster with ADDON_OPERATOR_NAMESPACE environment variable set",
			wantNamespace: "test-namespace",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.name {
			case "Running in-cluster":
				// Set the ADDON_OPERATOR_NAMESPACE environment variable
				os.Setenv("ADDON_OPERATOR_NAMESPACE", tt.wantNamespace)
				defer os.Unsetenv("ADDON_OPERATOR_NAMESPACE")

			case "Running outside cluster with ADDON_OPERATOR_NAMESPACE environment variable set":
				// Set the ADDON_OPERATOR_NAMESPACE environment variable
				os.Setenv("ADDON_OPERATOR_NAMESPACE", tt.wantNamespace)
				defer os.Unsetenv("ADDON_OPERATOR_NAMESPACE")

			}

			// Call the CurrentNamespace function and compare the result with the expected values
			gotNamespace, err := CurrentNamespace()
			if (err != nil) != tt.wantErr {
				t.Errorf("CurrentNamespace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotNamespace != tt.wantNamespace {
				t.Errorf("CurrentNamespace() = %v, want %v", gotNamespace, tt.wantNamespace)
			}
		})
	}
}
