package featuretoggle

import (
	"context"
	"os"
	"strings"

	"github.com/mt-sre/devkube/dev"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

type FeatureToggleHandler interface {
	Name() string
	GetFeatureToggleIdentifier() string
	PreManagerSetupHandle(ctx context.Context) error
	PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) error
	TestableFeatureToggleHandler
}

// to be used by tests / magefile to setup envs with / without feature toggles
type TestableFeatureToggleHandler interface {
	PreClusterCreationSetup(ctx context.Context) error
	PostClusterCreationSetup(ctx context.Context, clusterCreated *dev.Cluster) error
	Enable(ctx context.Context) error
	Disable(ctx context.Context) error
}

func IsEnabled(featureToggleHandlerToCheck FeatureToggleHandler, addonOperatorObjInCluster addonsv1alpha1.AddonOperator) bool {
	targetFeatureToggleIdentifier := featureToggleHandlerToCheck.GetFeatureToggleIdentifier()

	featureTogglesInClusterCommaSeparated := addonOperatorObjInCluster.Spec.FeatureToggles
	return stringPresentInSlice(targetFeatureToggleIdentifier, strings.Split(featureTogglesInClusterCommaSeparated, ","))
}

func IsEnabledOnTestEnv(featureToggleHandlerToCheck FeatureToggleHandler) bool {
	targetFeatureToggleIdentifier := featureToggleHandlerToCheck.GetFeatureToggleIdentifier()

	commaSeparatedFeatureToggles, ok := os.LookupEnv("FEATURE_TOGGLES")
	if !ok {
		return false
	}
	return stringPresentInSlice(targetFeatureToggleIdentifier, strings.Split(commaSeparatedFeatureToggles, ","))
}

func stringPresentInSlice(target string, slice []string) bool {
	for _, v := range slice {
		if v == target {
			return true
		}
	}
	return false
}
