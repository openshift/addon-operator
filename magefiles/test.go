//go:build mage
// +build mage

package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/go-logr/logr"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/mt-sre/devkube/dev"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	aoapisv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/featuretoggle"
)

type Test mg.Namespace

// Linting
func (Test) Lint() error {
	mg.Deps(
		Dependency.GolangciLint,
		Generate.All,
	)

	for _, cmd := range [][]string{
		{"go", "fmt", "./..."},
		{"bash", "./hack/validate-directory-clean.sh"},
		{"golangci-lint", "run", "./...", "--deadline=15m"},
	} {
		if err := sh.RunV(cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("running %q: %w", strings.Join(cmd, " "), err)
		}
	}
	return nil
}

// Unit tests
func (Test) Unit() error {
	return sh.RunWithV(map[string]string{
		// needed to enable race detector -race
		"CGO_ENABLED": "1",
	}, "go", "test", "-cover", "-v", "-race", "./internal/...", "./cmd/...", "./pkg/...")
}

// Integration tests

func (t Test) Integration(ctx context.Context) error { return t.integration(ctx, "") }

// Allows specifying a subset of tests to run e.g. ./mage test:integrationrun TestIntegration/TestPackageOperatorAddon
func (t Test) IntegrationRun(ctx context.Context, filter string) error {
	return t.integration(ctx, filter)
}

func (Test) integration(ctx context.Context, filter string) error {
	workDir, ok := ctx.Value("workDir").(string)
	if !ok || workDir == "" {
		workDir = path.Join(cacheDir, "dev-env")
	}
	cluster, err := dev.NewCluster(
		workDir,
		dev.WithKubeconfigPath(os.Getenv("KUBECONFIG")), dev.WithSchemeBuilder(runtime.SchemeBuilder{operatorsv1alpha1.AddToScheme, aoapisv1alpha1.AddToScheme}),
	)
	if err != nil {
		return fmt.Errorf("creating cluster client: %w", err)
	}

	// force ADDONS_PLUG_AND_PLAY feature toggle in CI to make sure tests are executed
	os.Setenv("FEATURE_TOGGLES", featuretoggle.AddonsPlugAndPlayFeatureToggleIdentifier)
	if err := postClusterCreationFeatureToggleSetup(ctx, cluster); err != nil {
		return fmt.Errorf("failed to perform post-cluster creation setup for the feature toggles: %w", err)
	}
	if err := deployFeatureToggles(ctx, cluster); err != nil {
		return fmt.Errorf("failed to deploy feature toggles: %w", err)
	}

	// will force a new run, instead of using the cache
	args := []string{"test", "-v", "-failfast", "-count=1", "-timeout=40m"}
	if len(filter) > 0 {
		args = append(args, "-run", filter)
	}
	args = append(args, "./integration/...")

	return sh.Run("go", args...)
}

// Target to prepare the CI-CD environment before installing the operator.
func (t Test) IntegrationCIPrepare(ctx context.Context) error {
	cluster, err := dev.NewCluster(path.Join(cacheDir, "ci"),
		dev.WithKubeconfigPath(os.Getenv("KUBECONFIG")))
	if err != nil {
		return fmt.Errorf("creating cluster client: %w", err)
	}

	ctx = logr.NewContext(ctx, logger)
	if err := labelNodesWithInfraRole(ctx, cluster); err != nil {
		return fmt.Errorf("failed to label the nodes with infra role: %w", err)
	}
	return nil
}

// Target to inject the addon status reporting environment variable
// to the addon operator CSV present in the cluster.
func (t Test) IntegrationCIInjectEnvVariable(ctx context.Context) error {
	cluster, err := dev.NewCluster(path.Join(cacheDir, "ci"),
		dev.WithKubeconfigPath(os.Getenv("KUBECONFIG")), dev.WithSchemeBuilder(operatorsv1alpha1.SchemeBuilder))
	if err != nil {
		return fmt.Errorf("creating cluster client: %w", err)
	}

	ctx = logr.NewContext(ctx, logger)
	if err = injectStatusReportingEnvironmentVariable(ctx, cluster); err != nil {
		return fmt.Errorf("inject ENV into CSV failed: %w", err)
	}
	return nil
}

func injectStatusReportingEnvironmentVariable(ctx context.Context, cluster *dev.Cluster) error {
	// OO_INSTALL_NAMESPACE is defined at:
	// https://github.com/openshift/release/blob/master/ci-operator/config/openshift/addon-operator/openshift-addon-operator-main.yaml#L79
	// The initial value("!create") is then over-written by the step https://steps.ci.openshift.org/reference/optional-operators-subscribe
	// to the actual namespace string.
	addonOperatorNS, found := os.LookupEnv("OO_INSTALL_NAMESPACE")
	if found && addonOperatorNS != "!create" {
		CSVList := &operatorsv1alpha1.ClusterServiceVersionList{}
		err := cluster.CtrlClient.List(ctx, CSVList, ctrlclient.InNamespace(addonOperatorNS))
		if err != nil {
			return fmt.Errorf("listing csv's for patching error: %w", err)
		}
		if len(CSVList.Items) == 0 {
			return fmt.Errorf("no csv's found in namespace: %s", addonOperatorNS)
		}

		addonOperatorCSV, found := findAddonOperatorCSV(CSVList)
		if !found {
			return fmt.Errorf("ADO csv missing in the OO_INSTALL_NAMESPACE(%s)", addonOperatorNS)
		}
		return patchAddonOperatorCSV(ctx, cluster, &addonOperatorCSV)
	}
	return fmt.Errorf("Invalid/Missing namespace value in OO_INSTALL_NAMESPACE: Got: %s", addonOperatorNS)
}

func findAddonOperatorCSV(csvList *operatorsv1alpha1.ClusterServiceVersionList) (operatorsv1alpha1.ClusterServiceVersion, bool) {
	for _, csv := range csvList.Items {
		if strings.HasPrefix(csv.Name, "addon-operator") {
			return csv, true
		}
	}
	return operatorsv1alpha1.ClusterServiceVersion{}, false
}

func patchAddonOperatorCSV(ctx context.Context,
	cluster *dev.Cluster,
	operatorCSV *operatorsv1alpha1.ClusterServiceVersion) error {
	for i := range operatorCSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		currentDeployment := &operatorCSV.
			Spec.
			InstallStrategy.
			StrategySpec.DeploymentSpecs[i]
		// Find the addon operator deployment.
		if currentDeployment.Name == "addon-operator-manager" {
			for i := range currentDeployment.Spec.Template.Spec.Containers {
				containerObj := &currentDeployment.Spec.Template.Spec.Containers[i]
				// Find the addon operator manager container from the pod.
				if containerObj.Name == "manager" {
					if containerObj.Env == nil {
						containerObj.Env = []corev1.EnvVar{}
					}
					// Set status reporting env variable to true.
					containerObj.Env = append(containerObj.Env, corev1.EnvVar{
						Name:  "ENABLE_STATUS_REPORTING",
						Value: "true"},
					)
					break
				}
			}
			break
		}
	}
	if err := cluster.CtrlClient.Update(ctx, operatorCSV); err != nil {
		return fmt.Errorf("patching ADO csv to inject env variable: %w", err)
	}
	// Wait for the newly patched deployment under the CSV to be ready
	deploymentObj := &appsv1.Deployment{}
	deploymentObj.SetName("addon-operator-manager")
	deploymentObj.SetNamespace(operatorCSV.Namespace)
	cluster.Waiter.WaitForObject(
		ctx,
		deploymentObj,
		"waiting for deployment to be ready and have status reporting env variable",
		func(obj ctrlclient.Object) (done bool, err error) {
			adoDeployment, ok := obj.(*appsv1.Deployment)
			if !ok {
				return false, fmt.Errorf("failed to type assert addon operator deployment")
			}
			// Wait for deployment to be ready
			if err := cluster.Waiter.WaitForReadiness(ctx, adoDeployment); err != nil {
				return false, nil
			}
			// Check if the deployment indeed has the env variable
			for _, container := range adoDeployment.Spec.Template.Spec.Containers {
				if container.Name == "manager" {
					for _, envObj := range container.Env {
						if envObj.Name == "ENABLE_STATUS_REPORTING" && envObj.Value == "true" {
							return true, nil
						}
					}
					break
				}
			}
			return false, nil
		},
	)
	return nil
}

func labelNodesWithInfraRole(ctx context.Context, cluster *dev.Cluster) error {
	nodeList := &corev1.NodeList{}
	if err := cluster.CtrlClient.List(ctx, nodeList); err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		node.Labels["node-role.kubernetes.io/infra"] = "True"
		if err := cluster.CtrlClient.Update(ctx, node); err != nil {
			return fmt.Errorf("adding infra role to all nodes: %w", err)
		}
	}
	return nil
}

// Target to run within OpenShift CI, where the Addon Operator and webhook is already deployed via the framework.
// This target will additionally deploy the API Mock before starting the integration test suite.
func (t Test) IntegrationCI(ctx context.Context) error {
	workDir := path.Join(cacheDir, "ci")
	cluster, err := dev.NewCluster(workDir,
		dev.WithKubeconfigPath(os.Getenv("KUBECONFIG")))
	if err != nil {
		return fmt.Errorf("creating cluster client: %w", err)
	}

	ctx = logr.NewContext(ctx, logger)

	var dev Dev
	if err := dev.deployAPIMock(ctx, cluster); err != nil {
		return fmt.Errorf("deploy API mock: %w", err)
	}

	os.Setenv("ENABLE_WEBHOOK", "true")
	os.Setenv("ENABLE_API_MOCK", "true")

	ctx = context.WithValue(ctx, "workDir", workDir)
	return t.Integration(ctx)
}

func (Test) IntegrationShort() error {
	return sh.Run("go", "test", "-v",
		"-count=1", // will force a new run, instead of using the cache
		"-short",
		"-timeout=20m", "./integration/...")
}
