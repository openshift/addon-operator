package featuretoggle

import (
	"context"

	"github.com/mt-sre/devkube/dev"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type FeatureToggleHandler interface {
	Name() string
	IsEnabled() bool
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
	IsEnabledOnTestEnv() bool
}
