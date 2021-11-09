package addon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers/common"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestEnsureWantedNamespaces_AddonWithoutNamespaces(t *testing.T) {
	c := testutil.NewClient()

	r := &AddonReconciler{
		Client: c,
		Log:    testutil.NewLogger(t),
		Scheme: common.NewTestSchemeWithAddonsv1alpha1(),
	}

	ctx := context.Background()
	stop, err := r.ensureWantedNamespaces(ctx, common.NewTestAddonWithoutNamespace())
	require.NoError(t, err)
	require.False(t, stop)
	c.AssertExpectations(t)
}

func TestEnsureWantedNamespaces_AddonWithSingleNamespace_Collision(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Run(func(args mock.Arguments) {
		arg := args.Get(2).(*corev1.Namespace)
		common.NewTestExistingNamespaceWithOwner().DeepCopyInto(arg)
	}).Return(nil)
	c.StatusMock.On("Update", testutil.IsContext, testutil.IsAddonsv1alpha1AddonPtr, mock.Anything).Return(nil)

	r := &AddonReconciler{
		Client: c,
		Log:    testutil.NewLogger(t),
		Scheme: common.NewTestSchemeWithAddonsv1alpha1(),
	}

	ctx := context.Background()
	stop, err := r.ensureWantedNamespaces(ctx, common.NewTestAddonWithSingleNamespace())
	require.NoError(t, err)
	require.True(t, stop)
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr)
	c.StatusMock.AssertCalled(t, "Update", testutil.IsContext, testutil.IsAddonsv1alpha1AddonPtr, mock.Anything)
}

func TestEnsureWantedNamespaces_AddonWithSingleNamespace_NoCollision(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Return(common.NewTestErrNotFound())
	c.On("Create", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).Run(func(args mock.Arguments) {
		arg := args.Get(1).(*corev1.Namespace)
		arg.Status = corev1.NamespaceStatus{
			Phase: corev1.NamespaceActive,
		}
	}).Return(nil)

	r := &AddonReconciler{
		Client: c,
		Log:    testutil.NewLogger(t),
		Scheme: common.NewTestSchemeWithAddonsv1alpha1(),
	}

	ctx := context.Background()
	stop, err := r.ensureWantedNamespaces(ctx, common.NewTestAddonWithSingleNamespace())
	require.NoError(t, err)
	require.False(t, stop)
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr)
	c.AssertCalled(t, "Create", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything)
}

func TestEnsureWantedNamespaces_AddonWithMultipleNamespaces_NoCollision(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Return(common.NewTestErrNotFound())
	c.On("Create", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).Run(func(args mock.Arguments) {
		arg := args.Get(1).(*corev1.Namespace)
		arg.Status = corev1.NamespaceStatus{
			Phase: corev1.NamespaceActive,
		}
	}).Return(nil)

	r := &AddonReconciler{
		Client: c,
		Log:    testutil.NewLogger(t),
		Scheme: common.NewTestSchemeWithAddonsv1alpha1(),
	}

	ctx := context.Background()
	stop, err := r.ensureWantedNamespaces(ctx, common.NewTestAddonWithMultipleNamespaces())
	require.NoError(t, err)
	require.False(t, stop)
	// every namespace should have been created
	namespaceCount := len(common.NewTestAddonWithMultipleNamespaces().Spec.Namespaces)
	c.AssertExpectations(t)
	c.AssertNumberOfCalls(t, "Get", namespaceCount)
	c.AssertNumberOfCalls(t, "Create", namespaceCount)
}

func TestEnsureWantedNamespaces_AddonWithMultipleNamespaces_SingleCollision(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).
		Return(common.NewTestErrNotFound()).
		Once()
	c.On("Create", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*corev1.Namespace)
			arg.Status = corev1.NamespaceStatus{
				Phase: corev1.NamespaceActive,
			}
		}).
		Return(nil).
		Once()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).
		Run(func(args mock.Arguments) {
			arg := args.Get(2).(*corev1.Namespace)
			common.NewTestExistingNamespaceWithOwner().DeepCopyInto(arg)
		}).
		Return(nil)
	c.StatusMock.On("Update", testutil.IsContext, testutil.IsAddonsv1alpha1AddonPtr, mock.Anything).
		Return(nil)

	r := &AddonReconciler{
		Client: c,
		Log:    testutil.NewLogger(t),
		Scheme: common.NewTestSchemeWithAddonsv1alpha1(),
	}

	ctx := context.Background()
	stop, err := r.ensureWantedNamespaces(ctx, common.NewTestAddonWithMultipleNamespaces())
	require.NoError(t, err)
	require.True(t, stop)
	c.AssertExpectations(t)
	c.AssertNumberOfCalls(t, "Get", len(common.NewTestAddonWithMultipleNamespaces().Spec.Namespaces))
	c.StatusMock.AssertCalled(t, "Update", testutil.IsContext, testutil.IsAddonsv1alpha1AddonPtr, mock.Anything)

}
func TestEnsureWantedNamespaces_AddonWithMultipleNamespaces_MultipleCollisions(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).
		Run(func(args mock.Arguments) {
			arg := args.Get(2).(*corev1.Namespace)
			common.NewTestExistingNamespaceWithOwner().DeepCopyInto(arg)
		}).
		Return(nil)
	c.StatusMock.On("Update", testutil.IsContext, testutil.IsAddonsv1alpha1AddonPtr, mock.Anything).
		Return(nil)

	r := &AddonReconciler{
		Client: c,
		Log:    testutil.NewLogger(t),
		Scheme: common.NewTestSchemeWithAddonsv1alpha1(),
	}

	ctx := context.Background()
	stop, err := r.ensureWantedNamespaces(ctx, common.NewTestAddonWithMultipleNamespaces())
	require.NoError(t, err)
	require.True(t, stop)
	c.AssertExpectations(t)
	c.AssertNumberOfCalls(t, "Get", len(common.NewTestAddonWithMultipleNamespaces().Spec.Namespaces))
	c.StatusMock.AssertCalled(t, "Update", testutil.IsContext, testutil.IsAddonsv1alpha1AddonPtr, mock.Anything)
}

func TestEnsureNamespace_Create(t *testing.T) {
	addon := common.NewTestAddonWithSingleNamespace()

	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Return(common.NewTestErrNotFound())
	c.On("Create", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).Return(nil)

	r := &AddonReconciler{
		Client: c,
		Log:    testutil.NewLogger(t),
		Scheme: common.NewTestSchemeWithAddonsv1alpha1(),
	}

	ctx := context.Background()
	ensuredNamespace, err := r.ensureNamespace(ctx, addon, addon.Spec.Namespaces[0].Name)
	c.AssertExpectations(t)
	require.NoError(t, err)
	require.NotNil(t, ensuredNamespace)
}

func TestReconcileNamespace_Create(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Return(common.NewTestErrNotFound())
	c.On("Create", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).Return(nil, common.NewTestNamespace())

	ctx := context.Background()
	reconciledNamespace, err := reconcileNamespace(ctx, c, common.NewTestSchemeWithAddonsv1alpha1(), common.NewTestNamespace(),
		addonsv1alpha1.ResourceAdoptionPrevent)
	require.NoError(t, err)
	assert.NotNil(t, reconciledNamespace)
	assert.Equal(t, common.NewTestNamespace(), reconciledNamespace)
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, client.ObjectKey{
		Name: "namespace-1",
	}, testutil.IsCoreV1NamespacePtr)
	c.AssertCalled(t, "Create", testutil.IsContext, common.NewTestNamespace(), mock.Anything)
}

func TestReconcileNamespace_CreateWithCollisionWithoutOwner(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Run(func(args mock.Arguments) {
		arg := args.Get(2).(*corev1.Namespace)
		common.NewTestExistingNamespaceWithoutOwner().DeepCopyInto(arg)
	}).Return(nil)

	ctx := context.Background()
	_, err := reconcileNamespace(ctx, c, common.NewTestSchemeWithAddonsv1alpha1(), common.NewTestNamespace(),
		addonsv1alpha1.ResourceAdoptionPrevent)
	require.EqualError(t, err, common.ErrNotOwnedByUs.Error())
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, client.ObjectKey{
		Name: "namespace-1",
	}, testutil.IsCoreV1NamespacePtr)
}

func TestReconcileNamespace_CreateWithCollisionWithOtherOwner(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Run(func(args mock.Arguments) {
		arg := args.Get(2).(*corev1.Namespace)
		common.NewTestExistingNamespaceWithoutOwner().DeepCopyInto(arg)
	}).Return(nil)

	ctx := context.Background()
	_, err := reconcileNamespace(ctx, c, common.NewTestSchemeWithAddonsv1alpha1(), common.NewTestNamespace(),
		addonsv1alpha1.ResourceAdoptionPrevent)
	require.EqualError(t, err, common.ErrNotOwnedByUs.Error())
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, client.ObjectKey{
		Name: "namespace-1",
	}, testutil.IsCoreV1NamespacePtr)
}

func TestReconcileNamespace_CreateWithClientError(t *testing.T) {
	timeoutErr := k8sApiErrors.NewTimeoutError("for testing", 1)

	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).
		Return(timeoutErr)

	ctx := context.Background()
	_, err := reconcileNamespace(ctx, c, common.NewTestSchemeWithAddonsv1alpha1(), common.NewTestNamespace(),
		addonsv1alpha1.ResourceAdoptionPrevent)
	require.Error(t, err)
	require.EqualError(t, err, timeoutErr.Error())
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, client.ObjectKey{
		Name: "namespace-1",
	}, testutil.IsCoreV1NamespacePtr)
}

func TestHasEqualControllerReference(t *testing.T) {
	require.True(t, HasEqualControllerReference(
		common.NewTestNamespace(),
		common.NewTestNamespace(),
	))

	require.False(t, HasEqualControllerReference(
		common.NewTestNamespace(),
		common.NewTestExistingNamespaceWithOwner(),
	))

	require.False(t, HasEqualControllerReference(
		common.NewTestNamespace(),
		common.NewTestExistingNamespaceWithoutOwner(),
	))
}
