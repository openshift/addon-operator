package controllers

import (
	"io/ioutil"
	"os"
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
// The TestAddCommonAnnotations function tests adding common annotation from an Addon
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
var getInClusterNamespacePath = func() string {
	return "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
}

func TestCurrentNamespace(t *testing.T) {
	tests := []struct {
		name          string
		envNamespace  string
		fileNamespace string
		wantNamespace string
		wantErr       bool
	}{
		{
			name:          "Namespace from Environment Variable",
			envNamespace:  "my-namespace",
			fileNamespace: "",
			wantNamespace: "my-namespace",
			wantErr:       false,
		},
		{
			name:          "Namespace from File",
			envNamespace:  "",
			fileNamespace: "my-namespace",
			wantNamespace: "my-namespace",
			wantErr:       false,
		},
		{
			name:          "No Namespace Specified",
			envNamespace:  "",
			fileNamespace: "",
			wantNamespace: "",
			wantErr:       true,
		},
	}

	// Override the function to provide a different value for the path
	getInClusterNamespacePath = func() string {
		return "mocked-namespace-file-path"
	}
	// Reset the function to its original implementation after the test
	defer func() {
		getInClusterNamespacePath = func() string {
			return "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
		}
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable if specified
			if tt.envNamespace != "" {
				os.Setenv("ADDON_OPERATOR_NAMESPACE", tt.envNamespace)
				defer os.Unsetenv("ADDON_OPERATOR_NAMESPACE")
			} else {
				os.Unsetenv("ADDON_OPERATOR_NAMESPACE")
			}

			// Create a temporary namespace file if specified
			if tt.fileNamespace != "" {
				tmpfile, err := ioutil.TempFile("", "namespace")
				if err != nil {
					t.Fatalf("Failed to create temporary namespace file: %v", err)
				}
				defer os.Remove(tmpfile.Name())

				if _, err := tmpfile.WriteString(tt.fileNamespace); err != nil {
					t.Fatalf("Failed to write to temporary namespace file: %v", err)
				}

				// Override the function to return the temporary file path
				getInClusterNamespacePath = func() string {
					return tmpfile.Name()
				}
			}

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
