package featuretoggle

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/mt-sre/devkube/dev"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var pkoVersion = "1.4.0"

func (handler *AddonsPlugAndPlayFeatureToggleHandler) PreClusterCreationSetup(ctx context.Context) error {
	return nil
}

func (handler *AddonsPlugAndPlayFeatureToggleHandler) PostClusterCreationSetup(ctx context.Context, clusterCreated *dev.Cluster) error {
	if err := clusterCreated.CreateAndWaitFromHttp(ctx, []string{
		"https://github.com/package-operator/package-operator/releases/download/v" + pkoVersion + "/self-bootstrap-job.yaml",
	}); err != nil {
		return fmt.Errorf("install PKO: %w", err)
	}

	deployment := &appsv1.Deployment{}
	deployment.SetNamespace("package-operator-system")
	deployment.SetName("package-operator-manager")

	if err := clusterCreated.Waiter.WaitForCondition(
		ctx, deployment, "Available", metav1.ConditionTrue,
		dev.WithInterval(10*time.Second), dev.WithTimeout(5*time.Minute),
	); err != nil {
		return fmt.Errorf("waiting for PKO installation: %w", err)
	}
	return nil
}
