package integration_test

import (
	"context"
	"fmt"
	"testing"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/addon-operator/integration"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestAddon_Subscription(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	uuid := "9c4a3192-6a79-4782-93dd-636e4d308852"
	addonName := fmt.Sprintf("addon-%s", uuid)
	addonNamespace := fmt.Sprintf("namespace-%s", uuid)
	addon := testutil.NewAddonOLMOwnNamespace(addonName, addonNamespace, referenceAddonCatalogSourceImageWorking)

	err := integration.Client.Create(ctx, addon)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := integration.Client.Delete(ctx, addon, client.PropagationPolicy("Foreground"))
		if client.IgnoreNotFound(err) != nil {
			t.Logf("could not clean up Addon %s: %v", addon.Name, err)
		}
	})

	err = integration.WaitForAddonToBeAvailable(t, defaultAddonAvailabilityTimeout, addon)
	require.NoError(t, err)

	subscription := &operatorsv1alpha1.Subscription{}
	{
		err := integration.Client.Get(ctx, client.ObjectKey{
			Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
			Name:      addon.Name,
		}, subscription)
		require.NoError(t, err)

		// Force type of `operatorsv1alpha1.SubscriptionStateAtLatest` to `operatorsv1alpha1.SubscriptionState`
		// because it is an untyped string const otherwise.
		var subscriptionAtLatest operatorsv1alpha1.SubscriptionState = operatorsv1alpha1.SubscriptionStateAtLatest
		assert.Equal(t, subscriptionAtLatest, subscription.Status.State)
		assert.NotEmpty(t, subscription.Status.Install)
		assert.Equal(t, "reference-addon.v0.1.0", subscription.Status.CurrentCSV)
		assert.Equal(t, "reference-addon.v0.1.0", subscription.Status.InstalledCSV)
	}

	{
		csv := &operatorsv1alpha1.ClusterServiceVersion{}
		err := integration.Client.Get(ctx, client.ObjectKey{
			Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
			Name:      subscription.Status.CurrentCSV,
		}, csv)
		require.NoError(t, err)

		assert.Equal(t, operatorsv1alpha1.CSVPhaseSucceeded, csv.Status.Phase)
	}

	// delete Addon
	err = integration.Client.Delete(ctx, addon, client.PropagationPolicy("Foreground"))
	require.NoError(t, err, "delete Addon: %v", addon)

	// wait until Addon is gone
	err = integration.WaitToBeGone(t, defaultAddonDeletionTimeout, addon)
	require.NoError(t, err, "wait for Addon to be deleted")

	// assert that CatalogSource is gone
	currentCatalogSource := &operatorsv1alpha1.CatalogSource{}
	err = integration.Client.Get(ctx, types.NamespacedName{
		Name:      addon.Name,
		Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
	}, currentCatalogSource)
	assert.True(t, k8sApiErrors.IsNotFound(err), "CatalogSource not deleted: %s", currentCatalogSource.Name)
}
