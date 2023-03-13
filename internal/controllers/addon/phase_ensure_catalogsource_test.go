package addon

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/equality"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/addon-operator/internal/controllers"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestReconcileCatalogSource_NotExistingYet_HappyPath(t *testing.T) {
	c := testutil.NewClient()
	c.On("Get",
		mock.Anything,
		testutil.IsObjectKey,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Return(testutil.NewTestErrNotFound())
	c.On("Create",
		mock.Anything,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Return(nil)

	ctx := context.Background()
	catalogSource := testutil.NewTestCatalogSource()
	reconciledCatalogSource, err := reconcileCatalogSource(ctx, c, catalogSource.DeepCopy())
	assert.NoError(t, err)
	assert.NotNil(t, reconciledCatalogSource)
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", mock.Anything, client.ObjectKey{
		Name:      catalogSource.Name,
		Namespace: catalogSource.Namespace,
	}, testutil.IsOperatorsV1Alpha1CatalogSourcePtr, mock.Anything)
	c.AssertCalled(t, "Create", mock.Anything,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr, mock.Anything)
}

func TestReconcileCatalogSource_NotExistingYet_WithClientErrorGet(t *testing.T) {
	timeoutErr := k8sApiErrors.NewTimeoutError("for testing", 1)

	c := testutil.NewClient()
	c.On("Get",
		mock.Anything,
		testutil.IsObjectKey,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Return(timeoutErr)

	ctx := context.Background()
	_, err := reconcileCatalogSource(ctx, c, testutil.NewTestCatalogSource())
	assert.Error(t, err)
	assert.EqualError(t, err, timeoutErr.Error())
	c.AssertExpectations(t)
}

func TestReconcileCatalogSource_NotExistingYet_WithClientErrorCreate(t *testing.T) {
	timeoutErr := k8sApiErrors.NewTimeoutError("for testing", 1)

	c := testutil.NewClient()
	c.On("Get",
		mock.Anything,
		testutil.IsObjectKey,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Return(testutil.NewTestErrNotFound())
	c.On("Create",
		mock.Anything,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Return(timeoutErr)

	ctx := context.Background()
	_, err := reconcileCatalogSource(ctx, c, testutil.NewTestCatalogSource())
	assert.Error(t, err)
	assert.EqualError(t, err, timeoutErr.Error())
	c.AssertExpectations(t)
}

func TestReconcileCatalogSource_Adoption(t *testing.T) {
	catalogSource := testutil.NewTestCatalogSource()

	c := testutil.NewClient()
	c.On("Get",
		mock.Anything,
		testutil.IsObjectKey,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Run(func(args mock.Arguments) {
		catalogSourceWithoutOwner := testutil.NewTestCatalogSourceWithoutOwner()
		catalogSourceWithoutOwner.DeepCopyInto(args.Get(2).(*operatorsv1alpha1.CatalogSource))
	}).Return(nil)

	c.On("Update",
		mock.Anything,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Return(nil)

	ctx := context.Background()

	reconciledCatalogSource, err := reconcileCatalogSource(ctx, c, catalogSource.DeepCopy())

	assert.NoError(t, err)
	assert.NotNil(t, reconciledCatalogSource)
	assert.True(t, equality.Semantic.DeepEqual(reconciledCatalogSource.OwnerReferences, catalogSource.OwnerReferences))
	c.AssertExpectations(t)
}

func TestEnsureCatalogSource_Create(t *testing.T) {
	addon := testutil.NewTestAddonWithCatalogSourceImage()

	c := testutil.NewClient()
	c.On("Get",
		mock.Anything,
		testutil.IsObjectKey,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Return(testutil.NewTestErrNotFound())

	var createdCatalogSource *operatorsv1alpha1.CatalogSource
	c.On("Create",
		mock.Anything,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Run(func(args mock.Arguments) {
		catalogSource := args.Get(1).(*operatorsv1alpha1.CatalogSource)
		catalogSource.Status.GRPCConnectionState = &operatorsv1alpha1.GRPCConnectionState{
			LastObservedState: "READY",
		}
		createdCatalogSource = catalogSource
	}).Return(nil)

	r := &olmReconciler{
		client: c,
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
	}

	log := testutil.NewLogger(t)

	ctx := controllers.ContextWithLogger(context.Background(), log)
	requeueResult, _, err := r.ensureCatalogSource(ctx, addon)
	assert.NoError(t, err)
	assert.Equal(t, resultNil, requeueResult)
	if c.AssertExpectations(t) {
		assert.Equal(t, []string{"test-pull-secret"}, createdCatalogSource.Spec.Secrets)
	}
}

func TestEnsureAdditionalCatalogSource_Create(t *testing.T) {
	addon := testutil.NewTestAddonWithAdditionalCatalogSources()
	c := testutil.NewClient()
	c.On("Get",
		mock.Anything,
		testutil.IsObjectKey,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Return(testutil.NewTestErrNotFound())
	c.On("Create",
		mock.Anything,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Run(func(args mock.Arguments) {
		catalogSource := args.Get(1).(*operatorsv1alpha1.CatalogSource)
		catalogSource.Status.GRPCConnectionState = &operatorsv1alpha1.GRPCConnectionState{
			LastObservedState: "READY",
		}
	}).Return(nil)
	r := &olmReconciler{
		client: c,
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
	}

	log := testutil.NewLogger(t)
	ctx := controllers.ContextWithLogger(context.Background(), log)
	requeueResult, err := r.ensureAdditionalCatalogSources(ctx, addon)
	assert.NoError(t, err)
	assert.Equal(t, resultNil, requeueResult)
	c.AssertExpectations(t)
	c.AssertNumberOfCalls(t, "Get", 2)
	c.AssertNumberOfCalls(t, "Create", 2)
}

func TestEnsureAdditionalCatalogSource_Update(t *testing.T) {
	addon := testutil.NewTestAddonWithAdditionalCatalogSources()
	c := testutil.NewClient()
	c.On("Get",
		mock.Anything,
		testutil.IsObjectKey,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Run(func(args mock.Arguments) {
		currentCatalogSource := args.Get(2).(*operatorsv1alpha1.CatalogSource)
		currentCatalogSource.Status.GRPCConnectionState = &operatorsv1alpha1.GRPCConnectionState{
			LastObservedState: "READY",
		}
	}).Return(nil)
	c.On("Update",
		mock.Anything,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Return(nil)
	r := &olmReconciler{
		client: c,
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
	}
	log := testutil.NewLogger(t)
	ctx := controllers.ContextWithLogger(context.Background(), log)
	requeueResult, err := r.ensureAdditionalCatalogSources(ctx, addon)
	assert.NoError(t, err)
	assert.Equal(t, resultNil, requeueResult)
	c.AssertExpectations(t)
	c.AssertNumberOfCalls(t, "Get", 2)
	c.AssertNumberOfCalls(t, "Update", 2)
}

func TestEnsureCatalogSource_Update(t *testing.T) {
	addon := testutil.NewTestAddonWithCatalogSourceImage()

	c := testutil.NewClient()
	c.On("Get",
		mock.Anything,
		testutil.IsObjectKey,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Run(func(args mock.Arguments) {
		currentCatalogSource := args.Get(2).(*operatorsv1alpha1.CatalogSource)
		currentCatalogSource.Status.GRPCConnectionState = &operatorsv1alpha1.GRPCConnectionState{
			LastObservedState: "READY",
		}
	}).Return(nil)
	c.On("Update",
		mock.Anything,
		testutil.IsOperatorsV1Alpha1CatalogSourcePtr,
		mock.Anything,
	).Return(nil)

	r := &olmReconciler{
		client: c,
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
	}

	log := testutil.NewLogger(t)
	ctx := controllers.ContextWithLogger(context.Background(), log)
	requeueResult, _, err := r.ensureCatalogSource(ctx, addon)

	assert.NoError(t, err)
	assert.Equal(t, resultNil, requeueResult)
	c.AssertExpectations(t)
	c.AssertNumberOfCalls(t, "Get", 1)
	c.AssertNumberOfCalls(t, "Update", 1)
}
