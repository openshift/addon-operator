package featuretoggle

import (
	"context"
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"

	"github.com/mt-sre/devkube/dev"
	"golang.org/x/exp/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

type Getter struct {
	Client                      client.Client
	SchemeToUpdate              *runtime.Scheme
	AddonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

func (g Getter) Get() []Handler {
	return []Handler{
		&MonitoringStackFeatureToggle{
			Client:                      g.Client,
			SchemeToUpdate:              g.SchemeToUpdate,
			AddonReconcilerOptsToUpdate: g.AddonReconcilerOptsToUpdate,
		},
		&AddonsPlugAndPlayFeatureToggle{
			Client:                      g.Client,
			SchemeToUpdate:              g.SchemeToUpdate,
			AddonReconcilerOptsToUpdate: g.AddonReconcilerOptsToUpdate,
		},
	}
}

type Handler interface {
	Name() string
	GetFeatureToggleIdentifier() string
	PreManagerSetupHandle(ctx context.Context) error
	PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) error
	TestableHandler
}

// to be used by tests / magefile to setup envs with / without feature toggles
type TestableHandler interface {
	PreClusterCreationSetup(ctx context.Context) error
	PostClusterCreationSetup(ctx context.Context, clusterCreated *dev.Cluster) error
	Enable(ctx context.Context) error
	Disable(ctx context.Context) error
}

func IsEnabled(featureToggleHandlerToCheck Handler, addonOperatorObjInCluster addonsv1alpha1.AddonOperator) bool {
	targetFeatureToggleIdentifier := featureToggleHandlerToCheck.GetFeatureToggleIdentifier()

	featureTogglesInClusterCommaSeparated := addonOperatorObjInCluster.Spec.FeatureToggles
	return slices.Contains(strings.Split(featureTogglesInClusterCommaSeparated, ","), targetFeatureToggleIdentifier)
}

func IsEnabledOnTestEnv(featureToggleHandlerToCheck Handler) bool {
	targetFeatureToggleIdentifier := featureToggleHandlerToCheck.GetFeatureToggleIdentifier()

	commaSeparatedFeatureToggles, ok := os.LookupEnv("FEATURE_TOGGLES")
	if !ok {
		return false
	}
	return slices.Contains(strings.Split(commaSeparatedFeatureToggles, ","), targetFeatureToggleIdentifier)
}

func EnableFeatureToggle(ctx context.Context, client client.Client, featureToggleIdentifier string) error {
	adoInCluster := addonsv1alpha1.AddonOperator{}
	if err := client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &adoInCluster); err != nil {
		if errors.IsNotFound(err) {
			adoObject := getAddonOperatorWithFeatureToggle(featureToggleIdentifier)
			if err := client.Create(ctx, &adoObject); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	// no need to do anything if its already enabled
	existingFeatureToggles := strings.Split(adoInCluster.Spec.FeatureToggles, ",")
	isFeatureToggleAlreadyEnabled := slices.Contains(existingFeatureToggles, featureToggleIdentifier)
	if isFeatureToggleAlreadyEnabled {
		return nil
	}
	newFeatureToggles := strings.Join([]string{adoInCluster.Spec.FeatureToggles, featureToggleIdentifier}, ",")
	adoInCluster.Spec.FeatureToggles = newFeatureToggles
	if err := client.Update(ctx, &adoInCluster); err != nil {
		return fmt.Errorf("failed to enable the feature toggle in the AddonOperator object: %w", err)
	}
	return nil
}

func DisableFeatureToggle(ctx context.Context, client client.Client, featureToggleIdentifier string) error {
	adoInCluster := addonsv1alpha1.AddonOperator{}
	if err := client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &adoInCluster); err != nil {
		if errors.IsNotFound(err) {
			adoObject := getAddonOperatorWithFeatureToggle("")
			if err := client.Create(ctx, &adoObject); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	// no need to do anything if its already disabled
	existingFeatureToggles := strings.Split(adoInCluster.Spec.FeatureToggles, ",")
	isAddonsPlugAndPlayAlreadyEnabled := slices.Contains(existingFeatureToggles, featureToggleIdentifier)
	if !isAddonsPlugAndPlayAlreadyEnabled {
		return nil
	}
	index := slices.Index(existingFeatureToggles, featureToggleIdentifier)
	newFeatureToggles := slices.Delete(existingFeatureToggles, index, index+1)
	adoInCluster.Spec.FeatureToggles = strings.Join(newFeatureToggles, ",")
	if err := client.Update(ctx, &adoInCluster); err != nil {
		return fmt.Errorf("failed to enable the feature toggle in the AddonOperator object: %w", err)
	}
	return nil
}

func getAddonOperatorWithFeatureToggle(featureToggle string) addonsv1alpha1.AddonOperator {
	adoObject := addonsv1alpha1.AddonOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: addonsv1alpha1.DefaultAddonOperatorName,
		},
		Spec: addonsv1alpha1.AddonOperatorSpec{
			FeatureToggles: featureToggle,
		},
	}
	return adoObject
}
