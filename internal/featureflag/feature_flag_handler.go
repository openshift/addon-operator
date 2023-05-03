package featureflag

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mt-sre/devkube/dev"
	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

type Getter struct {
	Client                      client.Client
	SchemeToUpdate              *runtime.Scheme
	AddonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

func (g Getter) Get() []Handler {
	return []Handler{
		&MonitoringStackFeatureFlag{
			Client:                      g.Client,
			SchemeToUpdate:              g.SchemeToUpdate,
			AddonReconcilerOptsToUpdate: g.AddonReconcilerOptsToUpdate,
		},
		&AddonsPlugAndPlayFeatureFlag{
			Client:                      g.Client,
			SchemeToUpdate:              g.SchemeToUpdate,
			AddonReconcilerOptsToUpdate: g.AddonReconcilerOptsToUpdate,
		},
	}
}

type Handler interface {
	Name() string
	GetFeatureFlagIdentifier() string
	PreManagerSetupHandle(ctx context.Context) error
	PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) error
	// Functions for integration test setup
	PreClusterCreationSetup(ctx context.Context) error
	PostClusterCreationSetup(ctx context.Context, clusterCreated *dev.Cluster) error
	Enable(ctx context.Context) error
	Disable(ctx context.Context) error
}

func IsEnabled(featureFlagHandler Handler, addonOperator addonsv1alpha1.AddonOperator) bool {
	targetFeatureFlagIdentifier := featureFlagHandler.GetFeatureFlagIdentifier()

	enabledFeatureFlags := addonOperator.Spec.FeatureFlags
	return slices.Contains(strings.Split(enabledFeatureFlags, ","), targetFeatureFlagIdentifier)
}

func IsEnabledOnTestEnv(featureFlagHandler Handler) bool {
	targetFeatureFlagIdentifier := featureFlagHandler.GetFeatureFlagIdentifier()

	commaSeparatedFeatureFlags, ok := os.LookupEnv("FEATURE_TOGGLES")
	if !ok {
		return false
	}
	return slices.Contains(strings.Split(commaSeparatedFeatureFlags, ","), targetFeatureFlagIdentifier)
}

func EnableFeatureFlag(ctx context.Context, client client.Client, featureFlagIdentifier string) error {
	ado := addonsv1alpha1.AddonOperator{}
	if err := client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &ado); err != nil {
		if errors.IsNotFound(err) {
			newAdo := getAddonOperatorWithFeatureFlag(featureFlagIdentifier)
			if err := client.Create(ctx, &newAdo); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	existingFeatureFlags := strings.Split(ado.Spec.FeatureFlags, ",")
	isFeatureFlagAlreadyEnabled := slices.Contains(existingFeatureFlags, featureFlagIdentifier)
	// no need to do anything if its already enabled
	if isFeatureFlagAlreadyEnabled {
		return nil
	}
	newFeatureFlags := strings.Join([]string{ado.Spec.FeatureFlags, featureFlagIdentifier}, ",")
	ado.Spec.FeatureFlags = newFeatureFlags
	if err := client.Update(ctx, &ado); err != nil {
		return fmt.Errorf("failed to enable the feature flag in the AddonOperator object: %w", err)
	}
	return nil
}

func DisableFeatureFlag(ctx context.Context, client client.Client, featureFlagIdentifier string) error {
	ado := addonsv1alpha1.AddonOperator{}
	if err := client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &ado); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		newAdo := getAddonOperatorWithFeatureFlag("")
		if err := client.Create(ctx, &newAdo); err != nil {
			return err
		}
		return nil
	}
	// no need to do anything if its already disabled
	existingFeatureFlags := strings.Split(ado.Spec.FeatureFlags, ",")
	isAddonsPlugAndPlayAlreadyEnabled := slices.Contains(existingFeatureFlags, featureFlagIdentifier)
	if !isAddonsPlugAndPlayAlreadyEnabled {
		return nil
	}
	index := slices.Index(existingFeatureFlags, featureFlagIdentifier)
	newFeatureFlags := slices.Delete(existingFeatureFlags, index, index+1)
	ado.Spec.FeatureFlags = strings.Join(newFeatureFlags, ",")
	if err := client.Update(ctx, &ado); err != nil {
		return fmt.Errorf("failed to disable the feature flag in the AddonOperator object: %w", err)
	}
	return nil
}

func getAddonOperatorWithFeatureFlag(featureFlag string) addonsv1alpha1.AddonOperator {
	adoObject := addonsv1alpha1.AddonOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: addonsv1alpha1.DefaultAddonOperatorName,
		},
		Spec: addonsv1alpha1.AddonOperatorSpec{
			FeatureFlags: featureFlag,
		},
	}
	return adoObject
}
