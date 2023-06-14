package controllers

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

// The TestCurrentNamespace function tests the behavior of the CurrentNamespace function.
// The CurrentNamespace function determines the current namespace based on environment 
// variables or a file.
func TestCurrentNamespace(t *testing.T) {
	// Test case: Local override with ADDON_OPERATOR_NAMESPACE environment variable
	t.Run("LocalOverride", func(t *testing.T) {
		expectedNamespace := "my-namespace"
		os.Setenv("ADDON_OPERATOR_NAMESPACE", expectedNamespace)
		defer os.Unsetenv("ADDON_OPERATOR_NAMESPACE")

		namespace, err := CurrentNamespace()
		require.NoError(t, err, "Unexpected error")
		require.Equal(t, expectedNamespace, namespace, "Namespace mismatch")
	})

	// Test case: Namespace file doesn't exist
	t.Run("FileNotExist", func(t *testing.T) {
		// Temporarily rename the namespace file to simulate its absence
		tmpfile, err := ioutil.TempFile("", "namespace")
		require.NoError(t, err, "Failed to create temporary namespace file")
		tmpfilePath := tmpfile.Name()
		tmpfile.Close()
		os.Rename(tmpfilePath, tmpfilePath+".backup")
		defer os.Rename(tmpfilePath+".backup", tmpfilePath)

		namespace, err := CurrentNamespace()
		require.Error(t, err, "Expected error")
		require.EqualError(t, err, "not running in-cluster, please specify ADDON_OPERATOR_NAMESPACE", "Error message mismatch")
		require.Equal(t, "", namespace, "Namespace should be empty")
	})

	// Test case: Error checking namespace file
	t.Run("ErrorCheckingFile", func(t *testing.T) {
		// Create a temporary directory and set it as the namespace file
		tmpDir, err := ioutil.TempDir("", "namespace")
		require.NoError(t, err, "Failed to create temporary directory")
		tmpfilePath := filepath.Join(tmpDir, "namespace")
		os.Setenv("ADDON_OPERATOR_NAMESPACE", tmpfilePath)
		defer os.Unsetenv("ADDON_OPERATOR_NAMESPACE")

		namespace, err := CurrentNamespace()
		require.Error(t, err, "Expected error")
		require.Contains(t, err.Error(), "error checking namespace file", "Error message mismatch")
		require.Equal(t, "", namespace, "Namespace should be empty")
	})

	// Test case: Successful namespace retrieval
	t.Run("Success", func(t *testing.T) {
		expectedNamespace := "my-namespace"

		// Create a temporary namespace file and write the expected namespace to it
		tmpfile, err := ioutil.TempFile("", "namespace")
		require.NoError(t, err, "Failed to create temporary namespace file")
		tmpfilePath := tmpfile.Name()
		defer os.Remove(tmpfilePath)

		_, err = tmpfile.WriteString(expectedNamespace)
		require.NoError(t, err, "Failed to write to temporary namespace file")

		namespace, err := CurrentNamespace()
		require.NoError(t, err, "Unexpected error")
		require.Equal(t, expectedNamespace, namespace, "Namespace mismatch")
	})
}