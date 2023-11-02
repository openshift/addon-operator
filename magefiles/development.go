//go:build mage
// +build mage

package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/magefile/mage/mg"
	"github.com/mt-sre/devkube/dev"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	kindv1alpha4 "sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/yaml"

	aoapisv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/featuretoggle"
)

type Dev mg.Namespace

var (
	devEnvironment *dev.Environment
)

func (d Dev) init() error {
	mg.SerialDeps(
		setupContainerRuntime,
		Dependency.Kind,
	)

	ctrl.SetLogger(logger)

	clusterInitializers := dev.WithClusterInitializers{
		dev.ClusterLoadObjectsFromHttp{
			// Install OLM.
			"https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v" + olmVersion + "/crds.yaml",
			"https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v" + olmVersion + "/olm.yaml",
		},
		dev.ClusterLoadObjectsFromFiles{
			// OCP APIs required by the AddonOperator.
			"config/ocp/cluster-version-operator_01_clusterversion.crd.yaml",
			"config/ocp/config-operator_01_proxy.crd.yaml",
			"config/ocp/cluster-version.yaml",
			"config/ocp/monitoring.coreos.com_servicemonitors.yaml",

			// OpenShift console to interact with OLM.
			"hack/openshift-console.yaml",
		},
	}

	devEnvironment = dev.NewEnvironment(
		"addon-operator-dev",
		path.Join(cacheDir, "dev-env"),
		dev.WithClusterOptions([]dev.ClusterOption{
			dev.WithWaitOptions([]dev.WaitOption{
				dev.WithTimeout(10 * time.Minute),
			}),
			dev.WithSchemeBuilder(k8sruntime.SchemeBuilder{operatorsv1alpha1.AddToScheme, aoapisv1alpha1.AddToScheme}),
		}),
		dev.WithContainerRuntime(containerRuntime),
		dev.WithKindClusterConfig(kindv1alpha4.Cluster{
			Nodes: []kindv1alpha4.Node{
				{
					Role: kindv1alpha4.ControlPlaneRole,
				},
				{
					Role: kindv1alpha4.WorkerRole,
				},
				{
					Role: kindv1alpha4.WorkerRole,
				},
			},
		}),
		clusterInitializers,
	)
	return nil
}

func (d Dev) Setup(ctx context.Context) error {
	if err := d.init(); err != nil {
		return err
	}

	if err := preClusterCreationFeatureToggleSetup(ctx); err != nil {
		return err
	}

	if err := devEnvironment.Init(ctx); err != nil {
		return fmt.Errorf("initializing dev environment: %w", err)
	}

	if err := postClusterCreationFeatureToggleSetup(ctx, devEnvironment.Cluster); err != nil {
		return err
	}

	return nil
}

func (d Dev) Teardown(ctx context.Context) error {
	if err := d.init(); err != nil {
		return err
	}

	if err := devEnvironment.Destroy(ctx); err != nil {
		return fmt.Errorf("tearing down dev environment: %w", err)
	}
	return nil
}

// Setup local dev environment with the addon operator installed and run the integration test suite.
func (d Dev) Integration(ctx context.Context) error {
	mg.SerialDeps(
		Dev.Deploy,
	)

	os.Setenv("KUBECONFIG", devEnvironment.Cluster.Kubeconfig())
	os.Setenv("ENABLE_WEBHOOK", "true")
	os.Setenv("ENABLE_API_MOCK", "true")
	os.Setenv("ENABLE_PROMETHEUS_REMOTE_STORAGE_MOCK", "true")
	os.Setenv("EXPERIMENTAL_FEATURES", "true")

	mg.SerialDeps(Test.Integration)
	return nil
}

func (d Dev) LoadImage(image string) error {
	mg.Deps(
		mg.F(Build.ImageBuild, image),
	)

	imageTar := path.Join(cacheDir, "image", image+".tar")
	if err := devEnvironment.LoadImageFromTar(imageTar); err != nil {
		return fmt.Errorf("load image from tar: %w", err)
	}
	return nil
}

// Deploy the Addon Operator, and additionally the Mock API Server and Addon Operator webhooks if the respective
// environment variables are set.
// All components are deployed via static manifests.
func (d Dev) Deploy(ctx context.Context) error {
	mg.Deps(
		Dev.Setup, // setup is a pre-requesite and needs to run before we can load images.
	)

	if err := labelNodesWithInfraRole(ctx, devEnvironment.Cluster); err != nil {
		return err
	}

	mg.Deps(
		mg.F(Dev.LoadImage, "api-mock"),
		mg.F(Dev.LoadImage, "addon-operator-manager"),
		mg.F(Dev.LoadImage, "addon-operator-webhook"),
		mg.F(Dev.LoadImage, "prometheus-remote-storage-mock"),
	)

	if err := d.deploy(ctx, devEnvironment.Cluster); err != nil {
		return fmt.Errorf("deploying: %w", err)
	}
	return nil
}

// Deploy all addon operator components to a cluster.
func (d Dev) deploy(
	ctx context.Context, cluster *dev.Cluster,
) error {
	if enableApiMock, ok := os.LookupEnv("ENABLE_API_MOCK"); ok &&
		enableApiMock == "true" {
		if err := d.deployAPIMock(ctx, cluster); err != nil {
			return err
		}
	}

	if err := d.deployAddonOperatorManager(ctx, cluster); err != nil {
		return err
	}

	if enableWebhooks, ok := os.LookupEnv("ENABLE_WEBHOOK"); ok &&
		enableWebhooks == "true" {
		if err := d.deployAddonOperatorWebhook(ctx, cluster); err != nil {
			return err
		}
	}

	if enablePrometheusRemoveStorageMock, ok := os.LookupEnv("ENABLE_PROMETHEUS_REMOTE_STORAGE_MOCK"); ok &&
		enablePrometheusRemoveStorageMock == "true" {
		if err := d.deployPrometheusRemoteStorageMock(ctx, cluster); err != nil {
			return err
		}
	}

	return nil
}

func renderPrometheusRemoteStorageMockDeployment(ctx context.Context, cluster *dev.Cluster) (*appsv1.Deployment, error) {
	objs, err := dev.LoadKubernetesObjectsFromFile("config/deploy/prometheus-remote-storage-mock/deployment.yaml.tpl")
	if err != nil {
		return nil, fmt.Errorf("failed to load the prometheus-remote-storage-mock deployment.yaml.tpl: %w", err)
	}

	// Replace image
	prometheusRemoteStorageMockDeployment := &appsv1.Deployment{}
	if err := cluster.Scheme.Convert(&objs[0], prometheusRemoteStorageMockDeployment, ctx); err != nil {
		return nil, fmt.Errorf("failed to convert the deployment: %w", err)
	}

	prometheusRemoteStorageMockImage := os.Getenv("PROMETHEUS_REMOTE_STORAGE_MOCK_IMAGE")
	if len(prometheusRemoteStorageMockImage) == 0 {
		prometheusRemoteStorageMockImage = imageURL("prometheus-remote-storage-mock")
	}
	for i := range prometheusRemoteStorageMockDeployment.Spec.Template.Spec.Containers {
		container := &prometheusRemoteStorageMockDeployment.Spec.Template.Spec.Containers[i]

		if container.Name == "mock" {
			container.Image = prometheusRemoteStorageMockImage
			break
		}
	}
	return prometheusRemoteStorageMockDeployment, nil
}

func (d Dev) deployPrometheusRemoteStorageMock(ctx context.Context, cluster *dev.Cluster) error {
	prometheusRemoteStorageMockDeployment, err := renderPrometheusRemoteStorageMockDeployment(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to render the prometheus remote storage mock deployment from its deployment template: %w", err)
	}

	if err := cluster.CreateAndWaitFromFiles(ctx, []string{
		"config/deploy/prometheus-remote-storage-mock/namespace.yaml",
		"config/deploy/prometheus-remote-storage-mock/service.yaml",
	}); err != nil {
		return fmt.Errorf("failed to load the prometheus-remote-storage-mock's namespace/service: %w", err)
	}

	if err := cluster.CreateAndWaitForReadiness(ctx, prometheusRemoteStorageMockDeployment); err != nil {
		return fmt.Errorf("failed to setup the prometheus-remote-storage-mock deployment: %w", err)
	}
	return nil
}

// deploy the API Mock server from local files.
func (d Dev) deployAPIMock(ctx context.Context, cluster *dev.Cluster) error {
	objs, err := dev.LoadKubernetesObjectsFromFile(
		"config/deploy/api-mock/deployment.yaml.tpl")
	if err != nil {
		return fmt.Errorf("loading api-mock deployment.yaml.tpl: %w", err)
	}

	// Replace image
	apiMockDeployment := &appsv1.Deployment{}
	if err := cluster.Scheme.Convert(
		&objs[0], apiMockDeployment, nil); err != nil {
		return fmt.Errorf("converting to Deployment: %w", err)
	}
	apiMockImage := os.Getenv("API_MOCK_IMAGE")
	if len(apiMockImage) == 0 {
		apiMockImage = imageURL("api-mock")
	}
	for i := range apiMockDeployment.Spec.Template.Spec.Containers {
		container := &apiMockDeployment.Spec.Template.Spec.Containers[i]

		switch container.Name {
		case "manager":
			container.Image = apiMockImage
		}
	}

	ctx = logr.NewContext(ctx, logger)

	// Deploy
	if err := cluster.CreateAndWaitFromFiles(ctx, []string{
		// TODO: replace with CreateAndWaitFromFolders when deployment.yaml is gone.
		"config/deploy/api-mock/00-namespace.yaml",
		"config/deploy/api-mock/api-mock.yaml",
	}); err != nil {
		return fmt.Errorf("deploy addon-operator-manager dependencies: %w", err)
	}
	if err := cluster.CreateAndWaitForReadiness(ctx, apiMockDeployment); err != nil {
		return fmt.Errorf("deploy api-mock: %w", err)
	}
	return nil
}

func deployFeatureToggles(ctx context.Context, cluster *dev.Cluster) error {
	availableFeatureToggles := featuretoggle.GetAvailableFeatureToggles(
		featuretoggle.WithClient{Client: cluster.CtrlClient},
		featuretoggle.WithSchemeToUpdate{Scheme: cluster.Scheme},
	)

	for _, featTog := range availableFeatureToggles {
		// feature toggles enabled/disabled at the level of openshift/release in the form of multiple jobs
		if featuretoggle.IsEnabledOnTestEnv(featTog) {
			if err := featTog.Enable(ctx); err != nil {
				return fmt.Errorf("failed to enable the feature toggle: %w", err)
			}
		} else {
			if err := featTog.Disable(ctx); err != nil {
				return fmt.Errorf("failed to disable the feature toggle: %w", err)
			}
		}
	}
	return nil
}

func preClusterCreationFeatureToggleSetup(ctx context.Context) error {
	availableFeatureToggles := featuretoggle.GetAvailableFeatureToggles()

	for _, featTog := range availableFeatureToggles {
		// feature toggles enabled/disabled at the level of openshift/release in the form of multiple jobs
		if featuretoggle.IsEnabledOnTestEnv(featTog) {
			if err := featTog.PreClusterCreationSetup(ctx); err != nil {
				return fmt.Errorf("failed to set the feature toggle before the cluster creation: %w", err)
			}
		}
	}
	return nil
}

func postClusterCreationFeatureToggleSetup(ctx context.Context, cluster *dev.Cluster) error {
	availableFeatureToggles := featuretoggle.GetAvailableFeatureToggles(
		featuretoggle.WithClient{Client: cluster.CtrlClient},
		featuretoggle.WithSchemeToUpdate{Scheme: cluster.Scheme},
	)

	for _, featTog := range availableFeatureToggles {
		// feature toggles enabled/disabled at the level of openshift/release in the form of multiple jobs
		if featuretoggle.IsEnabledOnTestEnv(featTog) {
			if err := featTog.PostClusterCreationSetup(ctx, cluster); err != nil {
				return fmt.Errorf("failed to set the feature toggle after the cluster creation: %w", err)
			}
		}
	}
	return nil
}

// deploy the Addon Operator Manager from local files.
func (d Dev) deployAddonOperatorManager(ctx context.Context, cluster *dev.Cluster) error {
	deployment := &appsv1.Deployment{}
	err := loadAndConvertIntoObject(cluster.Scheme, "config/deploy/deployment.yaml.tpl", deployment)
	if err != nil {
		return fmt.Errorf("loading addon-operator-manager deployment.yaml.tpl: %w", err)
	}

	// Replace image
	patchDeployment(deployment, "addon-operator-manager", "manager")

	ctx = logr.NewContext(ctx, logger)

	// Deploy
	if err := cluster.CreateAndWaitFromFiles(ctx, []string{
		// TODO: replace with CreateAndWaitFromFolders when deployment.yaml is gone.
		"config/deploy/00-namespace.yaml",
		"config/deploy/01-metrics-server-tls-secret.yaml",
		"config/deploy/addons.managed.openshift.io_addoninstances.yaml",
		"config/deploy/addons.managed.openshift.io_addonoperators.yaml",
		"config/deploy/addons.managed.openshift.io_addons.yaml",
		"config/deploy/metrics.service.yaml",
		"config/deploy/rbac.yaml",
		"config/deploy/trusted_ca_bundle_configmap.yaml",
	}); err != nil {
		return fmt.Errorf("deploy addon-operator-manager dependencies: %w", err)
	}

	if err := cluster.CreateAndWaitForReadiness(ctx, deployment); err != nil {
		return fmt.Errorf("deploy addon-operator-manager: %w", err)
	}
	if err := deployFeatureToggles(ctx, cluster); err != nil {
		return fmt.Errorf("deploy feature toggles: %w", err)
	}
	return nil
}

// Addon Operator Webhook server from local files.
func (d Dev) deployAddonOperatorWebhook(ctx context.Context, cluster *dev.Cluster) error {
	deployment := &appsv1.Deployment{}
	err := loadAndConvertIntoObject(cluster.Scheme, "config/deploy/webhook/deployment.yaml.tpl", deployment)
	if err != nil {
		return fmt.Errorf("loading addon-operator-webhook deployment.yaml.tpl: %w", err)
	}

	// Replace image
	patchDeployment(deployment, "addon-operator-webhook", "webhook")

	ctx = logr.NewContext(ctx, logger)

	// Deploy
	if err := cluster.CreateAndWaitFromFiles(ctx, []string{
		// TODO: replace with CreateAndWaitFromFolders when deployment.yaml is gone.
		"config/deploy/webhook/00-tls-secret.yaml",
		"config/deploy/webhook/service.yaml",
		"config/deploy/webhook/validatingwebhookconfig.yaml",
	}); err != nil {
		return fmt.Errorf("deploy addon-operator-webhook dependencies: %w", err)
	}
	if err := cluster.CreateAndWaitForReadiness(ctx, deployment); err != nil {
		return fmt.Errorf("deploy addon-operator-webhooks: %w", err)
	}
	return nil
}

// Replaces `container`'s image.
func patchDeployment(deployment *appsv1.Deployment, name string, container string) {
	image := getImageName(name)

	// replace image
	for i := range deployment.Spec.Template.Spec.Containers {
		containerObj := &deployment.Spec.Template.Spec.Containers[i]

		if containerObj.Name == container {
			containerObj.Image = image
			// Set status reporting env variable to true.
			containerObj.Env = []corev1.EnvVar{
				{
					Name:  "ENABLE_STATUS_REPORTING",
					Value: "true",
				},
			}
			break
		}
	}
}

func getImageName(name string) string {
	envVar := strings.ToUpper(name) + "_IMAGE"

	var image string
	if len(os.Getenv(envVar)) > 0 {
		image = os.Getenv(envVar)
	} else {
		image = imageURL(name)
	}
	return image
}

func loadAndConvertIntoObject(scheme *k8sruntime.Scheme, filePath string, out interface{}) error {
	objs, err := dev.LoadKubernetesObjectsFromFile(filePath)
	if err != nil {
		return fmt.Errorf("loading object from file: %w", err)
	}
	if err := scheme.Convert(&objs[0], out, nil); err != nil {
		return fmt.Errorf("converting: %w", err)
	}
	return nil
}

func loadAndUnmarshalIntoObject(filePath string, out interface{}) error {
	obj, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	if err = yaml.Unmarshal(obj, &out); err != nil {
		return err
	}
	return nil
}
