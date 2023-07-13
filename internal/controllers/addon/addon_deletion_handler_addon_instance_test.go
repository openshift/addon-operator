package addon

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestNotifyAddonStrategyNew(t *testing.T) {
	t.Run("no errors on addon instance not found error", func(t *testing.T) {
		client := testutil.NewClient()
		strategy := addonInstanceDeletionHandler{
			client: client,
		}
		addon := testutil.NewTestAddonWithCatalogSourceImage()
		client.On(
			"Get",
			mock.Anything,
			mock.Anything,
			mock.IsType(&addonsv1alpha1.AddonInstance{}),
			[]ctrlclient.GetOption(nil),
		).Return(testutil.NewTestErrNotFound())
		err := strategy.NotifyAddon(context.Background(), addon)
		client.AssertExpectations(t)
		require.NoError(t, err)
	})

	t.Run("updates addon instance's spec.MarkedForDeletion if its unset", func(t *testing.T) {
		client := testutil.NewClient()
		strategy := addonInstanceDeletionHandler{
			client: client,
		}
		addon := testutil.NewTestAddonWithCatalogSourceImage()
		client.On(
			"Get",
			mock.Anything,
			mock.Anything,
			mock.IsType(&addonsv1alpha1.AddonInstance{}),
			[]ctrlclient.GetOption(nil),
		).Return(nil)
		client.On(
			"Update",
			mock.Anything,
			mock.IsType(&addonsv1alpha1.AddonInstance{}),
			[]ctrlclient.UpdateOption(nil),
		).Return(nil)

		// Assertions
		err := strategy.NotifyAddon(context.Background(), addon)
		require.NoError(t, err)
		client.AssertCalled(
			t,
			"Update",
			mock.Anything,
			mock.MatchedBy(func(instance *addonsv1alpha1.AddonInstance) bool {
				return instance.Spec.MarkedForDeletion == true
			}),
			[]ctrlclient.UpdateOption(nil),
		)
		client.AssertExpectations(t)
	})

	t.Run("doesnt update if spec.MarkedForDeletion is already true", func(t *testing.T) {
		client := testutil.NewClient()
		strategy := addonInstanceDeletionHandler{
			client: client,
		}
		addon := testutil.NewTestAddonWithCatalogSourceImage()
		existingInstance := addonsv1alpha1.AddonInstance{
			Spec: addonsv1alpha1.AddonInstanceSpec{
				MarkedForDeletion: true,
			},
		}
		client.On(
			"Get",
			mock.Anything,
			mock.Anything,
			mock.IsType(&addonsv1alpha1.AddonInstance{}),
			[]ctrlclient.GetOption(nil),
		).Run(func(args mock.Arguments) {
			instance := args.Get(2).(*addonsv1alpha1.AddonInstance)
			*instance = existingInstance
		}).Return(nil)

		// Assertions
		err := strategy.NotifyAddon(context.Background(), addon)
		require.NoError(t, err)
		client.AssertNotCalled(t, "Update")
		client.AssertExpectations(t)
	})
}

func TestAckReceivedFromAddonStrategyNew(t *testing.T) {
	testCases := []struct {
		addonInstanceGetErr error
		addonInstance       *addonsv1alpha1.AddonInstance
		shouldReceiveAck    bool
		errPresent          bool
	}{
		{
			addonInstanceGetErr: testutil.NewTestErrNotFound(),
			addonInstance:       nil,
			shouldReceiveAck:    false,
			errPresent:          false,
		},
		{
			addonInstanceGetErr: errors.New("kubeapi busy"),
			addonInstance:       nil,
			shouldReceiveAck:    false,
			errPresent:          true,
		},
		// condition not present
		{
			addonInstanceGetErr: nil,
			addonInstance:       &addonsv1alpha1.AddonInstance{},
			shouldReceiveAck:    false,
			errPresent:          false,
		},
		{
			addonInstanceGetErr: nil,
			addonInstance: &addonsv1alpha1.AddonInstance{
				Status: addonsv1alpha1.AddonInstanceStatus{
					Conditions: []v1.Condition{
						{
							Type:   addonsv1alpha1.AddonInstanceConditionReadyToBeDeleted.String(),
							Status: v1.ConditionUnknown,
						},
					},
				},
			},
			shouldReceiveAck: false,
			errPresent:       false,
		},
		{
			addonInstanceGetErr: nil,
			addonInstance: &addonsv1alpha1.AddonInstance{
				Status: addonsv1alpha1.AddonInstanceStatus{
					Conditions: []v1.Condition{
						{
							Type:   addonsv1alpha1.AddonInstanceConditionReadyToBeDeleted.String(),
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			shouldReceiveAck: true,
			errPresent:       false,
		},
	}

	for _, tc := range testCases {
		addon := testutil.NewTestAddonWithCatalogSourceImage()
		client := testutil.NewClient()
		strategy := addonInstanceDeletionHandler{
			client: client,
		}

		// Setup mocks
		if tc.addonInstanceGetErr != nil {
			client.On(
				"Get",
				mock.Anything,
				mock.Anything,
				mock.IsType(&addonsv1alpha1.AddonInstance{}),
				[]ctrlclient.GetOption(nil),
			).Return(tc.addonInstanceGetErr)
		} else {
			client.On(
				"Get",
				mock.Anything,
				mock.Anything,
				mock.IsType(&addonsv1alpha1.AddonInstance{}),
				[]ctrlclient.GetOption(nil),
			).Run(func(args mock.Arguments) {
				instance := args.Get(2).(*addonsv1alpha1.AddonInstance)
				*instance = *tc.addonInstance
			}).Return(nil)
		}

		ackReceived, err := strategy.AckReceivedFromAddon(context.Background(), addon)

		require.Equal(t, tc.shouldReceiveAck, ackReceived)
		if tc.errPresent {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}

}
