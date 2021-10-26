package integration_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/addon-operator/integration"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestNamespaceCreation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	addon := testutil.NewAddonOLMOwnNamespace(
		"addon-c01m94lbi",
		"namespace-oibabdsoi",
		referenceAddonCatalogSourceImageWorking,
	)

	err := integration.Client.Create(ctx, addon)
	require.NoError(t, err)

	// clean up addon resource in case it
	// was leaked because of a failed test
	wasAlreadyDeleted := false
	defer func() {
		if !wasAlreadyDeleted {
			err := integration.Client.Delete(ctx, addon)
			if err != nil {
				t.Logf("could not clean up object %s: %v", addon.Name, err)
			}
		}
	}()

	// wait until Addon is available
	err = integration.WaitForAddonToBeAvailable(t, defaultAddonAvailabilityTimeout, addon)
	require.NoError(t, err)

	// validate Namespaces
	for _, namespace := range addon.Spec.Namespaces {
		currentNamespace := &corev1.Namespace{}
		err := integration.Client.Get(ctx, types.NamespacedName{
			Name: namespace.Name,
		}, currentNamespace)
		assert.NoError(t, err, "could not get Namespace %s", namespace.Name)

		assert.Equal(t, currentNamespace.Status.Phase, corev1.NamespaceActive)
	}

	// delete Addon
	err = integration.Client.Delete(ctx, addon, client.PropagationPolicy("Foreground"))
	require.NoError(t, err, "delete Addon: %v", addon)

	// wait until Addon is gone
	err = integration.WaitToBeGone(t, defaultAddonDeletionTimeout, addon)
	require.NoError(t, err, "wait for Addon to be deleted")

	wasAlreadyDeleted = true

	// assert that all Namespaces are gone
	for _, namespace := range addon.Spec.Namespaces {
		currentNamespace := &corev1.Namespace{}
		err := integration.Client.Get(ctx, types.NamespacedName{
			Name: namespace.Name,
		}, currentNamespace)
		assert.True(t, k8sApiErrors.IsNotFound(err), "Namespace not deleted: %s", namespace.Name)
	}
}
