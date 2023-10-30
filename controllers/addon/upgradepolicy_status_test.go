package addon

import (
	"context"
	"testing"

	"github.com/openshift/addon-operator/internal/metrics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/internal/ocm"
	"github.com/openshift/addon-operator/internal/ocm/ocmtest"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestAddonReconciler_handleUpgradePolicyStatusReporting(t *testing.T) {
	t.Run("noop without .spec.upgradePolicy", func(t *testing.T) {
		r := &AddonReconciler{}
		log := testutil.NewLogger(t)

		err := r.handleUpgradePolicyStatusReporting(
			context.Background(),
			log,
			&addonsv1alpha1.Addon{},
		)
		require.NoError(t, err)
	})

	t.Run("noop when upgrade already completed", func(t *testing.T) {
		r := &AddonReconciler{}
		log := testutil.NewLogger(t)

		err := r.handleUpgradePolicyStatusReporting(
			context.Background(),
			log,
			&addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					UpgradePolicy: &addonsv1alpha1.AddonUpgradePolicy{
						ID: "1234",
					},
				},
				Status: addonsv1alpha1.AddonStatus{
					UpgradePolicy: &addonsv1alpha1.AddonUpgradePolicyStatus{
						ID:    "1234",
						Value: addonsv1alpha1.AddonUpgradePolicyValueCompleted,
					},
				},
			},
		)
		require.NoError(t, err)
	})

	t.Run("noop when OCM client is missing", func(t *testing.T) {
		r := &AddonReconciler{}
		log := testutil.NewLogger(t)

		err := r.handleUpgradePolicyStatusReporting(
			context.Background(),
			log,
			&addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					UpgradePolicy: &addonsv1alpha1.AddonUpgradePolicy{
						ID: "1234",
					},
				},
			},
		)
		require.NoError(t, err)
	})

	t.Run("post `started` on new upgradePolicyID", func(t *testing.T) {
		client := testutil.NewClient()
		ocmClient := ocmtest.NewClient()

		recorder := metrics.NewRecorder(false, "asa346546dfew143")
		mockSummary := testutil.NewSummaryMock()
		recorder.InjectOCMAPIRequestDuration(mockSummary)

		r := &AddonReconciler{
			Client:    client,
			ocmClient: ocmClient,
			Recorder:  recorder,
		}

		var Version = "1.0.0"

		log := testutil.NewLogger(t)
		addon := &addonsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 100,
			},
			Spec: addonsv1alpha1.AddonSpec{
				Version: Version,
				UpgradePolicy: &addonsv1alpha1.AddonUpgradePolicy{
					ID: "1234",
				},
			},
		}

		ocmClient.
			On("GetUpgradePolicy", mock.Anything, ocm.UpgradePolicyGetRequest{
				ID: "1234",
			}).
			Return(
				ocm.UpgradePolicyGetResponse{
					Value: ocm.UpgradePolicyValueScheduled,
				}, nil,
			)

		ocmClient.
			On("PatchUpgradePolicy", mock.Anything, ocm.UpgradePolicyPatchRequest{
				ID:          "1234",
				Value:       ocm.UpgradePolicyValueStarted,
				Description: `Upgrading addon to version "1.0.0".`,
			}).
			Return(
				ocm.UpgradePolicyPatchResponse{},
				nil,
			)

		mockSummary.On(
			"Observe", mock.IsType(float64(0)))

		err := r.handleUpgradePolicyStatusReporting(
			context.Background(), log, addon)
		require.NoError(t, err)

		mockSummary.AssertExpectations(t)
		ocmClient.AssertExpectations(t)
		client.AssertExpectations(t)

		if assert.NotNil(t, addon.Status.UpgradePolicy) {
			assert.Equal(t, "1234", addon.Status.UpgradePolicy.ID)
			assert.Equal(t,
				addonsv1alpha1.AddonUpgradePolicyValueStarted,
				addon.Status.UpgradePolicy.Value)
			assert.Equal(t,
				addon.Generation,
				addon.Status.UpgradePolicy.ObservedGeneration)
		}
	})

	t.Run("noop when upgrade started, but Addon not Available", func(t *testing.T) {
		ocmClient := ocmtest.NewClient()
		ocmClient.
			On("GetUpgradePolicy", mock.Anything, ocm.UpgradePolicyGetRequest{
				ID: "1234",
			}).
			Return(
				ocm.UpgradePolicyGetResponse{
					Value: ocm.UpgradePolicyValueStarted,
				}, nil,
			)

		r := &AddonReconciler{
			ocmClient: ocmClient,
		}
		log := testutil.NewLogger(t)

		var Version = "1.0.0"

		err := r.handleUpgradePolicyStatusReporting(
			context.Background(),
			log,
			&addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Version: Version,
					UpgradePolicy: &addonsv1alpha1.AddonUpgradePolicy{
						ID: "1234",
					},
				},
				Status: addonsv1alpha1.AddonStatus{
					UpgradePolicy: &addonsv1alpha1.AddonUpgradePolicyStatus{
						ID:      "1234",
						Version: Version,
						Value:   addonsv1alpha1.AddonUpgradePolicyValueStarted,
					},
				},
			},
		)
		require.NoError(t, err)
	})

	t.Run("post `completed` after `started` when Available", func(t *testing.T) {
		client := testutil.NewClient()
		ocmClient := ocmtest.NewClient()
		recorder := metrics.NewRecorder(false, "asdfew143")

		mockSummary := testutil.NewSummaryMock()
		recorder.InjectOCMAPIRequestDuration(mockSummary)

		var Version = "1.0.0"

		r := &AddonReconciler{
			Client:    client,
			ocmClient: ocmClient,
			Recorder:  recorder,
		}
		log := testutil.NewLogger(t)
		addon := &addonsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 100,
			},
			Spec: addonsv1alpha1.AddonSpec{
				Version: Version,
				UpgradePolicy: &addonsv1alpha1.AddonUpgradePolicy{
					ID: "1234",
				},
			},
			Status: addonsv1alpha1.AddonStatus{
				Conditions: []metav1.Condition{
					{
						Type:   addonsv1alpha1.AddonOperatorAvailable,
						Status: metav1.ConditionTrue,
					},
				},
				UpgradePolicy: &addonsv1alpha1.AddonUpgradePolicyStatus{
					ID:      "1234",
					Version: Version,
					Value:   addonsv1alpha1.AddonUpgradePolicyValueStarted,
				},
			},
		}

		ocmClient.
			On("GetUpgradePolicy", mock.Anything, ocm.UpgradePolicyGetRequest{
				ID: "1234",
			}).
			Return(
				ocm.UpgradePolicyGetResponse{
					Value: ocm.UpgradePolicyValueStarted,
				}, nil,
			)
		ocmClient.
			On("PatchUpgradePolicy", mock.Anything, ocm.UpgradePolicyPatchRequest{
				ID:          "1234",
				Value:       ocm.UpgradePolicyValueCompleted,
				Description: `Addon was healthy at least once at version "1.0.0".`,
			}).
			Return(
				ocm.UpgradePolicyPatchResponse{},
				nil,
			)
		mockSummary.On(
			"Observe", mock.IsType(float64(0)))

		err := r.handleUpgradePolicyStatusReporting(
			context.Background(), log, addon)
		require.NoError(t, err)

		mockSummary.AssertExpectations(t)
		ocmClient.AssertExpectations(t)
		client.AssertExpectations(t)

		if assert.NotNil(t, addon.Status.UpgradePolicy) {
			assert.Equal(t, "1234", addon.Status.UpgradePolicy.ID)
			assert.Equal(t,
				addonsv1alpha1.AddonUpgradePolicyValueCompleted,
				addon.Status.UpgradePolicy.Value)
			assert.Equal(t,
				addon.Generation,
				addon.Status.UpgradePolicy.ObservedGeneration)
		}
	})
}
