package addon_test

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/addon-operator/apis/addons/v1alpha1"
	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers/addon"
	"github.com/openshift/addon-operator/internal/testutil"
)

var (
	addonWithoutPKO = testutil.NewTestAddonWithSingleNamespace()
	addonWithPKO    = &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "test-addon", Namespace: "test-ns"},
		Spec:       addonsv1alpha1.AddonSpec{AddonPackageOperator: &v1alpha1.AddonPackageOperator{Image: "test-pko-img"}},
	}
)

type subTest struct {
	// function that configures mock client for the specific test
	configureClient func(context.Context, *testutil.Client, types.NamespacedName)
	// test asserts for Error or NotError depending on this boolean
	expectError bool
}

func TestPackageOperatorReconcilerName(t *testing.T) {
	c := testutil.NewClient()
	r := &addon.PackageOperatorReconciler{Client: c, Scheme: testutil.NewTestSchemeWithAddonsv1alpha1()}
	require.Equal(t, "packageOperatorReconciler", r.Name())
}

func TestPackageOperatorReconcilerLogic(t *testing.T) {
	for name, test := range map[string]struct {
		addon    *v1alpha1.Addon
		subTests map[string]subTest
	}{
		"ClusterObjectTemplateMustExist": {
			addonWithPKO,
			map[string]subTest{
				"Found": {
					func(ctx context.Context, c *testutil.Client, identifier types.NamespacedName) {
						c.On("Get", ctx, identifier, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), []client.GetOption(nil)).
							Return(nil).
							Once()
						c.On("Patch", ctx, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), mock.Anything, []client.PatchOption(nil)).
							Return(nil).
							Once()
					},
					false,
				},
				"NotFound": {
					func(ctx context.Context, c *testutil.Client, identifier types.NamespacedName) {
						c.On("Get", ctx, identifier, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), []client.GetOption(nil)).
							Return(errors.NewNotFound(schema.GroupResource{}, "test")).
							Once()
						c.On("Create", ctx, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), []client.CreateOption(nil)).
							Return(nil).
							Once()
					},
					false,
				},
				"Error": {
					func(ctx context.Context, c *testutil.Client, identifier types.NamespacedName) {
						errIn := io.ErrClosedPipe
						c.
							On("Get", ctx, identifier, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), []client.GetOption(nil)).
							Return(errors.NewNotFound(schema.GroupResource{}, "test")).
							Once().
							Return(errIn).
							Once()
					},
					true,
				},
			},
		},
		"ClusterObjectTemplateMustNotExist": {
			addonWithoutPKO,
			map[string]subTest{
				"Found": {
					func(ctx context.Context, c *testutil.Client, identifier types.NamespacedName) {
						c.On("Get", ctx, identifier, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), []client.GetOption(nil)).
							Return(nil).
							Once()
						c.On("Delete", ctx, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), []client.DeleteOption(nil)).
							Return(nil).
							Once()
					},
					false,
				},
				"NotFound": {
					func(ctx context.Context, c *testutil.Client, identifier types.NamespacedName) {
						c.On("Get", ctx, identifier, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), []client.GetOption(nil)).
							Return(errors.NewNotFound(schema.GroupResource{}, "test")).
							Once()
					},
					false,
				},
				"Error": {
					func(ctx context.Context, c *testutil.Client, identifier types.NamespacedName) {
						errIn := io.ErrClosedPipe
						c.
							On("Get", ctx, identifier, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), []client.GetOption(nil)).
							Return(errors.NewNotFound(schema.GroupResource{}, "test")).
							Once().
							Return(errIn).
							Once()
					},
					true,
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			for subName, subTest := range test.subTests {
				t.Run(subName, func(t *testing.T) {
					c := testutil.NewClient()

					identifier := types.NamespacedName{Namespace: test.addon.Namespace, Name: test.addon.Name}

					ctx := context.Background()
					subTest.configureClient(ctx, c, identifier)

					r := &addon.PackageOperatorReconciler{Client: c, Scheme: testutil.NewTestSchemeWithAddonsv1alpha1()}

					res, err := r.Reconcile(ctx, test.addon)

					if subTest.expectError {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
					require.Equal(t, reconcile.Result{}, res)
					c.AssertExpectations(t)
				})
			}
		})
	}
}
