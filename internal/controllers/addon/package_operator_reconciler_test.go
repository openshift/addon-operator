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

func TestPackageOperatorReconciler_Name(t *testing.T) {
	c := testutil.NewClient()
	r := &addon.PackageOperatorReconciler{Client: c, Scheme: testutil.NewTestSchemeWithAddonsv1alpha1()}
	require.Equal(t, "packageOperatorReconciler", r.Name())
}

func TestPackageOperatorReconciler_Exists(t *testing.T) {
	c := testutil.NewClient()

	image := "quayeiei"
	namespace := "testnamespace"
	addonname := "addonname"
	identifier := types.NamespacedName{Namespace: namespace, Name: addonname}

	a := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: addonname, Namespace: namespace},
		Spec:       addonsv1alpha1.AddonSpec{AddonPackageOperator: &v1alpha1.AddonPackageOperator{Image: image}},
	}

	ctx := context.Background()
	c.On("Get", ctx, identifier, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), []client.GetOption(nil)).Return(nil).Once()
	c.On("Patch", ctx, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), mock.Anything, []client.PatchOption(nil)).Return(nil).Once()

	r := &addon.PackageOperatorReconciler{Client: c, Scheme: testutil.NewTestSchemeWithAddonsv1alpha1()}

	res, err := r.Reconcile(ctx, a)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)
	c.AssertExpectations(t)
}

func TestPackageOperatorReconciler_NotFound(t *testing.T) {
	c := testutil.NewClient()

	image := "quayeiei"
	namespace := "testnamespace"
	addonname := "addonname"
	identifier := types.NamespacedName{Namespace: namespace, Name: addonname}

	a := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: addonname, Namespace: namespace},
		Spec:       addonsv1alpha1.AddonSpec{AddonPackageOperator: &v1alpha1.AddonPackageOperator{Image: image}},
	}

	ctx := context.Background()
	c.On("Get", ctx, identifier, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), []client.GetOption(nil)).Return(errors.NewNotFound(schema.GroupResource{}, "test")).Once()
	c.On("Create", ctx, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), []client.CreateOption(nil)).Return(nil).Once()

	r := &addon.PackageOperatorReconciler{Client: c, Scheme: testutil.NewTestSchemeWithAddonsv1alpha1()}

	res, err := r.Reconcile(ctx, a)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)
	c.AssertExpectations(t)
}

func TestPackageOperatorReconciler_Err(t *testing.T) {
	c := testutil.NewClient()

	image := "quayeiei"
	namespace := "testnamespace"
	addonname := "addonname"
	identifier := types.NamespacedName{Namespace: namespace, Name: addonname}

	a := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: addonname, Namespace: namespace},
		Spec:       addonsv1alpha1.AddonSpec{AddonPackageOperator: &v1alpha1.AddonPackageOperator{Image: image}},
	}

	errIn := io.ErrClosedPipe
	ctx := context.Background()
	c.On("Get", ctx, identifier, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), []client.GetOption(nil)).Return(errors.NewNotFound(schema.GroupResource{}, "test")).Once().Return(errIn).Once()

	r := &addon.PackageOperatorReconciler{Client: c, Scheme: testutil.NewTestSchemeWithAddonsv1alpha1()}

	res, err := r.Reconcile(ctx, a)
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
	c.AssertExpectations(t)
}

func TestPackageOperatorReconciler_NoPKO(t *testing.T) {
	c := testutil.NewClient()
	a := testutil.NewTestAddonWithSingleNamespace()
	r := &addon.PackageOperatorReconciler{Client: c, Scheme: testutil.NewTestSchemeWithAddonsv1alpha1()}

	ctx := context.Background()
	result, err := r.Reconcile(ctx, a)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, result)
	c.AssertExpectations(t)
}
