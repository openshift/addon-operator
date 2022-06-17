package addon

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestEnsureWantedNamespaces_AddonWithoutNamespaces(t *testing.T) {
	c := testutil.NewClient()

	r := &namespaceReconciler{
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		client: c,
	}

	ctx := context.Background()
	err := r.ensureWantedNamespaces(ctx, testutil.NewTestAddonWithoutNamespace())
	require.NoError(t, err)
	c.AssertExpectations(t)
}

func TestEnsureWantedNamespaces_AddonWithSingleNamespace_Adoption(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Run(func(args mock.Arguments) {
		arg := args.Get(2).(*corev1.Namespace)
		testutil.NewTestExistingNamespace().DeepCopyInto(arg)
	}).Return(nil)
	c.On("Update", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).Run(func(args mock.Arguments) {
		arg := args.Get(1).(*corev1.Namespace)
		arg.Status.Phase = corev1.NamespaceActive
	}).Return(nil)
	r := &namespaceReconciler{
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		client: c,
	}

	ctx := context.Background()
	addon := testutil.NewTestAddonWithSingleNamespace()
	err := r.ensureWantedNamespaces(ctx, addon)
	require.NoError(t, err)
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr)

	// validate Status condition
	availableCond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.Available)
	assert.Nil(t, availableCond)
}

func TestEnsureWantedNamespaces_AddonWithSingleNamespace_Create(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Return(testutil.NewTestErrNotFound())
	c.On("Create", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).Run(func(args mock.Arguments) {
		arg := args.Get(1).(*corev1.Namespace)
		arg.Status = corev1.NamespaceStatus{
			Phase: corev1.NamespaceActive,
		}
	}).Return(nil)

	r := &namespaceReconciler{
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		client: c,
	}

	ctx := context.Background()
	err := r.ensureWantedNamespaces(ctx, testutil.NewTestAddonWithSingleNamespace())
	require.NoError(t, err)
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr)
	c.AssertCalled(t, "Create", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything)
}

func TestEnsureWantedNamespaces_AddonWithMultipleNamespaces_Create(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Return(testutil.NewTestErrNotFound())
	c.On("Create", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).Run(func(args mock.Arguments) {
		arg := args.Get(1).(*corev1.Namespace)
		arg.Status = corev1.NamespaceStatus{
			Phase: corev1.NamespaceActive,
		}
	}).Return(nil)

	r := &namespaceReconciler{
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		client: c,
	}

	ctx := context.Background()
	err := r.ensureWantedNamespaces(ctx, testutil.NewTestAddonWithMultipleNamespaces())
	require.NoError(t, err)
	// every namespace should have been created
	namespaceCount := len(testutil.NewTestAddonWithMultipleNamespaces().Spec.Namespaces)
	c.AssertExpectations(t)
	c.AssertNumberOfCalls(t, "Get", namespaceCount)
	c.AssertNumberOfCalls(t, "Create", namespaceCount)
}

func TestEnsureWantedNamespaces_AddonWithMultipleNamespaces_SingleAdoption(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).
		Return(testutil.NewTestErrNotFound()).
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
			testutil.NewTestExistingNamespace().DeepCopyInto(arg)
		}).
		Return(nil)
	c.On("Update", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).
		Return(nil).
		Once()

	r := &namespaceReconciler{
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		client: c,
	}

	ctx := context.Background()
	addon := testutil.NewTestAddonWithMultipleNamespaces()
	addonCopy := addon.DeepCopy()
	err := r.ensureWantedNamespaces(ctx, addon)
	require.NoError(t, err)
	c.AssertExpectations(t)
	c.AssertNumberOfCalls(t, "Get", len(addonCopy.Spec.Namespaces))
	c.AssertNumberOfCalls(t, "Create", 1)
	c.AssertNumberOfCalls(t, "Update", 1)

}
func TestEnsureWantedNamespaces_AddonWithMultipleNamespaces_MultipleAdoptions(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).
		Run(func(args mock.Arguments) {
			arg := args.Get(2).(*corev1.Namespace)
			testutil.NewTestExistingNamespace().DeepCopyInto(arg)
		}).
		Return(nil)
	c.On("Update", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).
		Return(nil)

	r := &namespaceReconciler{
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		client: c,
	}

	ctx := context.Background()
	addon := testutil.NewTestAddonWithMultipleNamespaces()
	addonCopy := addon.DeepCopy()
	err := r.ensureWantedNamespaces(ctx, addon)
	require.NoError(t, err)
	c.AssertExpectations(t)
	c.AssertNumberOfCalls(t, "Get", len(addonCopy.Spec.Namespaces))
	c.AssertNumberOfCalls(t, "Update", len(addonCopy.Spec.Namespaces))
}

func TestEnsureNamespace_Create(t *testing.T) {
	addon := testutil.NewTestAddonWithSingleNamespace()

	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Return(testutil.NewTestErrNotFound())
	c.On("Create", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).Return(nil)

	r := &namespaceReconciler{
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		client: c,
	}

	ctx := context.Background()
	ensuredNamespace, err := r.ensureNamespace(ctx, addon, addon.Spec.Namespaces[0].Name)
	c.AssertExpectations(t)
	require.NoError(t, err)
	require.NotNil(t, ensuredNamespace)
}

func TestEnsureNamespace_CreateWithLabels(t *testing.T) {
	addon := testutil.NewTestAddonWithSingleNamespace()
	labels := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Return(testutil.NewTestErrNotFound())
	c.On("Create", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).
		Run(func(args mock.Arguments) {
			ns := args.Get(1).(*corev1.Namespace)
			for key, value := range labels {
				assert.Equal(t, value, ns.Labels[key])
			}
		}).
		Return(nil)

	r := &namespaceReconciler{
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		client: c,
	}

	ctx := context.Background()

	ensuredNamespace, err := r.ensureNamespaceWithLabels(ctx, addon, addon.Spec.Namespaces[0].Name, labels)
	c.AssertExpectations(t)
	require.NoError(t, err)
	require.NotNil(t, ensuredNamespace)
}

func TestReconcileNamespace_Create(t *testing.T) {
	namespace := testutil.NewTestNamespace()

	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Return(testutil.NewTestErrNotFound())
	c.On("Create", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).Return(nil, namespace)

	ctx := context.Background()
	reconciledNamespace, err := reconcileNamespace(ctx, c, namespace)
	require.NoError(t, err)
	assert.NotNil(t, reconciledNamespace)
	assert.Equal(t, namespace, reconciledNamespace)
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, client.ObjectKey{
		Name: namespace.Name,
	}, testutil.IsCoreV1NamespacePtr)
	c.AssertCalled(t, "Create", testutil.IsContext, namespace, mock.Anything)
}

func TestReconcileNamespace_CreateWithAdoptionWithoutOwner(t *testing.T) {
	namespace := testutil.NewTestNamespace()

	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Run(func(args mock.Arguments) {
		arg := args.Get(2).(*corev1.Namespace)
		testutil.NewTestExistingNamespaceWithoutOwner().DeepCopyInto(arg)
	}).Return(nil)
	c.On("Update",
		testutil.IsContext,
		testutil.IsCoreV1NamespacePtr,
		mock.Anything,
	).Return(nil)

	ctx := context.Background()
	_, err := reconcileNamespace(ctx, c, namespace)

	assert.Nil(t, err)
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, client.ObjectKey{
		Name: namespace.Name,
	}, testutil.IsCoreV1NamespacePtr)
}

func TestReconcileNamespace_CreateWithAdoptionWithOtherOwner(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Run(func(args mock.Arguments) {
		arg := args.Get(2).(*corev1.Namespace)
		testutil.NewTestExistingNamespaceWithoutOwner().DeepCopyInto(arg)
	}).Return(nil)
	c.On("Update",
		testutil.IsContext,
		testutil.IsCoreV1NamespacePtr,
		mock.Anything,
	).Return(nil)

	ctx := context.Background()
	namespace := testutil.NewTestNamespace()
	namespaceCopy := namespace.DeepCopy()
	_, err := reconcileNamespace(ctx, c, namespace)

	assert.Nil(t, err)
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, client.ObjectKey{
		Name: namespaceCopy.Name,
	}, testutil.IsCoreV1NamespacePtr)
}

func TestReconcileNamespace_CreateWithClientError(t *testing.T) {
	timeoutErr := k8sApiErrors.NewTimeoutError("for testing", 1)

	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).
		Return(timeoutErr)

	ctx := context.Background()
	namespace := testutil.NewTestNamespace()
	namespaceCopy := namespace.DeepCopy()
	_, err := reconcileNamespace(ctx, c, namespace)
	require.Error(t, err)
	require.EqualError(t, err, timeoutErr.Error())
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, client.ObjectKey{
		Name: namespaceCopy.Name,
	}, testutil.IsCoreV1NamespacePtr)
}

func TestEnsureDeletionOfUnwantedNamespaces_NoNamespacesInSpec_NoNamespacesInCluster(t *testing.T) {
	c := testutil.NewClient()

	c.On("List", testutil.IsContext, testutil.IsCoreV1NamespaceListPtr, mock.Anything).
		Return(nil)

	r := &namespaceReconciler{
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		client: c,
	}

	ctx := context.Background()
	err := r.ensureDeletionOfUnwantedNamespaces(ctx, testutil.NewTestAddonWithoutNamespace())
	require.NoError(t, err)
	c.AssertExpectations(t)
}

func TestEnsureDeletionOfUnwantedNamespaces_NoNamespacesInSpec_NamespaceInCluster(t *testing.T) {
	c := testutil.NewClient()

	addon := testutil.NewTestAddonWithoutNamespace()
	existingNamespace := testutil.NewTestNamespace()

	c.On("List", testutil.IsContext, testutil.IsCoreV1NamespaceListPtr, mock.Anything).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*corev1.NamespaceList)
			namespaceList := corev1.NamespaceList{
				Items: []corev1.Namespace{
					*existingNamespace,
				},
			}
			namespaceList.DeepCopyInto(arg)
		}).
		Return(nil)
	c.On("Delete", testutil.IsContext, testutil.IsCoreV1NamespacePtr, mock.Anything).
		Return(nil)

	r := &namespaceReconciler{
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		client: c,
	}

	ctx := context.Background()
	err := r.ensureDeletionOfUnwantedNamespaces(ctx, addon)
	require.NoError(t, err)
	c.AssertExpectations(t)
	c.AssertCalled(t, "List", testutil.IsContext,
		testutil.IsCoreV1NamespaceListPtr,
		// verify that the list call did use the correct labelSelector
		mock.MatchedBy(func(listOptions []client.ListOption) bool {
			testListOptions := &client.ListOptions{}
			listOptions[0].ApplyToList(testListOptions)
			testLabelSelectorString := testListOptions.LabelSelector.String()
			return len(testLabelSelectorString) > 0 &&
				testLabelSelectorString == controllers.CommonLabelsAsLabelSelector(addon).String()
		}))
	c.AssertCalled(t, "Delete", testutil.IsContext,
		mock.MatchedBy(func(val *corev1.Namespace) bool {
			return val.Name == existingNamespace.Name
		}),
		mock.Anything)
}

func TestEnsureDeletionOfUnwantedNamespaces_NamespacesInSpec_matching_NamespacesInCluster(t *testing.T) {
	c := testutil.NewClient()

	addon := testutil.NewTestAddonWithSingleNamespace()
	existingNamespace := testutil.NewTestNamespace()

	c.On("List", testutil.IsContext, testutil.IsCoreV1NamespaceListPtr, mock.Anything).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*corev1.NamespaceList)
			namespaceList := corev1.NamespaceList{
				Items: []corev1.Namespace{
					*existingNamespace,
				},
			}
			namespaceList.DeepCopyInto(arg)
		}).
		Return(nil)

	r := &namespaceReconciler{
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		client: c,
	}

	ctx := context.Background()
	err := r.ensureDeletionOfUnwantedNamespaces(ctx, addon)
	require.NoError(t, err)
	c.AssertExpectations(t)
	c.AssertCalled(t, "List", testutil.IsContext,
		testutil.IsCoreV1NamespaceListPtr,
		// verify that the list call did use the correct labelSelector
		mock.MatchedBy(func(listOptions []client.ListOption) bool {
			testListOptions := &client.ListOptions{}
			listOptions[0].ApplyToList(testListOptions)
			testLabelSelectorString := testListOptions.LabelSelector.String()
			return len(testLabelSelectorString) > 0 &&
				testLabelSelectorString == controllers.CommonLabelsAsLabelSelector(addon).String()
		}))
}

func TestEnsureDeletionOfUnwantedNamespaces_NoNamespacesInSpec_WithClientError(t *testing.T) {
	timeoutErr := k8sApiErrors.NewTimeoutError("for testing", 1)

	c := testutil.NewClient()
	c.On("List", testutil.IsContext, testutil.IsCoreV1NamespaceListPtr, mock.Anything).
		Return(timeoutErr)

	r := &namespaceReconciler{
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		client: c,
	}

	ctx := context.Background()
	err := r.ensureDeletionOfUnwantedNamespaces(ctx, testutil.NewTestAddonWithoutNamespace())
	require.EqualError(t, errors.Unwrap(err), timeoutErr.Error())
	c.AssertExpectations(t)
}
