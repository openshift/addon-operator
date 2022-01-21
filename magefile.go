//go:build mage
// +build mage

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	appsv1 "k8s.io/api/apps/v1"

	"github.com/mt-sre/devkube/dev"
	"github.com/mt-sre/devkube/magedeps"
)

// Dependency Versions
const (
	controllerGenVersion = "0.6.2"
	kindVersion          = "0.11.1"
	yqVersion            = "4.12.0"
	goimportsVersion     = "0.1.5"
	golangciLintVersion  = "1.43.0"
	olmVersion           = "0.19.1"
	opmVersion           = "1.18.0"
	helmVersion          = "3.7.2"
)

const (
	kindClusterName = "addon-operator"
)

// Directories
var (
	// Working directory of the project.
	workDir string
	// Cache directory for temporary build files.
	cacheDir string
	// Dependency directory.
	depsDir magedeps.DependencyDirectory
)

// Build Tags
var (
	branch        string
	shortCommitID string
	version       string
	buildDate     string

	ldFlags string
)

// Runtime Configuration
var (
	// podman or docker
	containerRuntime string
	imageOrg         string
)

// Development Environments
var (
	defaultDevEnvironment *dev.Environment
	logger                logr.Logger
)

func init() {
	var err error
	// Directories
	workDir, err = os.Getwd()
	if err != nil {
		panic(fmt.Errorf("getting work dir: %w", err))
	}

	depsDir = magedeps.DependencyDirectory(path.Join(workDir, ".deps"))
	cacheDir = path.Join(workDir + ".cache")

	// Path
	os.Setenv("PATH", depsDir.Bin()+":"+os.Getenv("PATH"))

	// Build Tags
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchBytes, err := branchCmd.Output()
	if err != nil {
		panic(fmt.Errorf("getting git branch: %w", err))
	}
	branch = strings.TrimSpace(string(branchBytes))

	shortCommitIDCmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	shortCommitIDBytes, err := shortCommitIDCmd.Output()
	if err != nil {
		panic(fmt.Errorf("getting git short commit id"))
	}
	shortCommitID = strings.TrimSpace(string(shortCommitIDBytes))

	version = strings.TrimSpace(os.Getenv("VERSION"))
	if len(version) == 0 {
		version = shortCommitID
	}

	buildDate = fmt.Sprint(time.Now().UTC().Unix())
	module := "github.com/openshift/addon-operator"
	ldFlags = fmt.Sprintf(`-X %s/internal/version.Version=%s`+
		`-X %s/internal/version.Branch=%s`+
		`-X %s/internal/version.Commit=%s`+
		`-X %s/internal/version.BuildDate=%s`,
		module, version,
		module, branch,
		module, shortCommitID,
		module, buildDate,
	)

	// Runtime
	containerRuntime = os.Getenv("CONTAINER_RUNTIME")
	if len(containerRuntime) == 0 {
		containerRuntime = "podman"
	}
	imageOrg = os.Getenv("IMAGE_ORG")
	if len(imageOrg) == 0 {
		imageOrg = "quay.io/app-sre"
	}

	// Development Environments
	defaultDevEnvironment = dev.NewEnvironment(
		"addon-operator-dev",
		path.Join(cacheDir, "dev-env"),
		dev.WithContainerRuntime(containerRuntime),
		dev.WithClusterInitializers{
			dev.ClusterLoadObjectsFromFiles{
				// OCP APIs required by the AddonOperator.
				"config/ocp/cluster-version-operator_01_clusterversion.crd.yaml",
				"config/ocp/config-operator_01_proxy.crd.yaml",
				"config/ocp/cluster-version.yaml",
				"config/ocp/monitoring.coreos.com_servicemonitors.yaml",

				// OpenShift console to interact with OLM.
				"hack/openshift-console.yaml",
			},
			dev.ClusterLoadObjectsFromHttp{
				// Install OLM.
				"https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v" + olmVersion + "/crds.yaml",
				"https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v" + olmVersion + "/olm.yaml",
			},
			dev.ClusterHelmInstall{
				RepoName:    "prometheus-community",
				RepoURL:     "https://prometheus-community.github.io/helm-charts",
				PackageName: "kube-prometheus-stack",
				ReleaseName: "prometheus",
				Namespace:   "monitoring",
				SetVars: []string{
					"grafana.enabled=false",
					"kubeStateMetrics.enabled=false",
					"nodeExporter.enabled=false",
				},
			},
		})

	logger = stdr.New(log.Default())
}

// Runs code gens for deepcopy, kubernetes manifests and docs.
func Generate() {
	mg.Deps(
		Generators.code,
		Generators.docs,
		Generators.openshiftCITestBuild,
	)
}

// Runs go mod tidy in all go modules of the repository.
func Tidy() error {
	apisTidyCmd := exec.Command("go", "mod", "tidy")
	apisTidyCmd.Dir = path.Join(workDir, "apis")
	if err := apisTidyCmd.Run(); err != nil {
		return fmt.Errorf("tidy apis module: %w", err)
	}

	if err := sh.Run("go", "mod", "tidy"); err != nil {
		return fmt.Errorf("tidy main module: %w", err)
	}

	return nil
}

// Build
// -----
type Build mg.Namespace

// Default build target for CI/CD
func (Build) All() {
	mg.Deps(
		mg.F(Build.cmdWithGOARGS, "addon-operator-manager", "linux", "amd64"),
		mg.F(Build.cmdWithGOARGS, "addon-operator-webhook", "linux", "amd64"),
		mg.F(Build.cmdWithGOARGS, "api-mock", "linux", "amd64"),
		mg.F(Build.cmd, "mage"),
	)
}

// Builds the docgen internal tool
func (Build) Docgen() {
	mg.Deps(mg.F(Build.cmd, "docgen"))
}

// Builds binaries from /cmd directory.
func (Build) cmdWithGOARGS(cmd, goos, goarch string) error {
	mg.Deps(
		Generators.code,
	)

	env := map[string]string{
		"GOFLAGS":     "",
		"CGO_ENABLED": "0",
		"LDFLAGS":     ldFlags,
	}
	bin := path.Join("bin", cmd)
	if len(goos) != 0 && len(goarch) != 0 {
		bin = path.Join("bin", goos+"_"+goarch, cmd)
		env["GOARGS"] = fmt.Sprintf("GOOS=%s GOARCH=%s", goos, goarch)
	}

	if err := sh.RunWithV(
		env,
		"go", "build", "-v", "-o", bin, "./cmd/"+cmd+"/main.go",
	); err != nil {
		return fmt.Errorf("compiling cmd/%s: %w", cmd, err)
	}
	return nil
}

// Builds binaries from /cmd directory.
func (b Build) cmd(cmd string) error {
	return b.cmdWithGOARGS(cmd, "", "")
}

func (Build) image(cmd string) error {
	mg.Deps(
		mg.F(Build.cmd, cmd),
	)

	imageCacheDir := path.Join(cacheDir, "image", cmd)
	if err := os.RemoveAll(imageCacheDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting image cache: %w", err)
	}
	if err := os.Remove(imageCacheDir + ".tar"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting image cache: %w", err)
	}
	if err := os.MkdirAll(imageCacheDir, os.ModePerm); err != nil {
		return fmt.Errorf("create image cache dir: %w", err)
	}

	imageTag := imageOrg + "/" + cmd + ":" + version
	for _, copy := range [][]string{
		// Copy files for build environment
		{"cp", "-a",
			"bin/linux_amd64/" + cmd,
			imageCacheDir + "/" + cmd},
		{"cp", "-a",
			"config/docker/" + cmd + ".Dockerfile",
			imageCacheDir + "/Dockerfile"},

		// Build image!
		{containerRuntime, "build", "-t", imageTag, imageCacheDir},
		{containerRuntime, "image", "save",
			"-o", imageCacheDir + ".tar", imageTag},
	} {
		if err := sh.Run(copy[0], copy[1:]...); err != nil {
			return fmt.Errorf("running %q: %w", strings.Join(copy, " "), err)
		}
	}

	return nil
}

func (Build) imagePush(imageName string) error {
	mg.Deps(
		mg.F(Build.image, imageName),
	)

	// Login to container registry when running on AppSRE Jenkins.
	if _, ok := os.LookupEnv("JENKINS_HOME"); ok {
		log.Println("running in Jenkins, calling container runtime login")
		if err := sh.Run(containerRuntime,
			"login", "-u="+os.Getenv("QUAY_USER"),
			"-p="+os.Getenv("QUAY_TOKEN"), "quay.io"); err != nil {
			return fmt.Errorf("registry login: %w", err)
		}
	}

	imageTag := imageOrg + "/" + imageName + ":" + version
	if err := sh.Run(containerRuntime, "push", imageTag); err != nil {
		return fmt.Errorf("pushing image: %w", err)
	}

	return nil
}

// Testing and Linting
// -------------------

// Runs code-generators, checks for clean directory and lints the source code.
func Lint() error {
	mg.Deps(Generate, Dependency.GolangciLint)

	for _, cmd := range [][]string{
		{"go", "fmt", "./..."},
		{"bash", "hack/validate-directory-clean.sh"},
		{"golangci-lint", "run", "./...", "--deadline=15m"},
	} {
		if err := sh.RunV(cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("running %q: %w", strings.Join(cmd, " "), err)
		}
	}
	return nil
}

type Test mg.Namespace

// Runs code-generators and unittests.
func (Test) Unit() error {
	mg.Deps(Generate)

	return sh.RunWithV(map[string]string{
		"CGO_ENABLED": "1",
	}, "go", "test", "-v", "-race", "./internal/...", "./cmd/...")
}

// Runs the Integration testsuite against the current $KUBECONFIG cluster.
func (Test) Integration() error {
	return sh.RunWithV(map[string]string{
		"ENABLE_WEBHOOK":  "true",
		"ENABLE_API_MOCK": "true",
	}, "go", "test", "-v", "-count=1", "-timeout=20m", "./integration/...")
}

func (Test) IntegrationShort() error {
	return sh.RunV(
		"go", "test", "-v", "-count=1", "-short", "./integration/...")
}

// CI/CD specific
// --------------
type CICD mg.Namespace

// Deploy the API Mock to run the integration test suite.
func (CICD) DeployAPIMock(ctx context.Context) error {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	cluster, err := dev.NewCluster(path.Base(kubeconfigPath), dev.WithKubeconfigPath(kubeconfigPath))
	if err != nil {
		return err
	}

	return deployAPIMock(ctx, cluster)
}

// Development
// -----------
type Dev mg.Namespace

func (Dev) init(ctx context.Context) error {
	mg.Deps(Dependency.kind)

	if err := defaultDevEnvironment.Init(ctx); err != nil {
		return fmt.Errorf("initializing default dev environment: %w", err)
	}
	return nil
}

// Setup just an empty kubernetes cluster.
func (Dev) Empty() {
	mg.Deps(Dev.init)
}

// Deploy the Addon Operator, Mock API Server and Addon Operator webhooks (if env ENABLE_WEBHOOK=true) is set.
// All components are deployed via static manifests.
func (Dev) Deploy(ctx context.Context) error {
	if err := deploy(ctx, defaultDevEnvironment.Cluster); err != nil {
		return fmt.Errorf("deploying: %w", err)
	}
	return nil
}

// deploy the API Mock server from local files.
func deployAPIMock(ctx context.Context, cluster *dev.Cluster) error {
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
		apiMockImage = imageOrg + "/api-mock:" + version
	}
	apiMockDeployment.Spec.Template.Spec.Containers[0].Image = apiMockImage
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

// deploy the Addon Operator Manager from local files.
func deployAddonOperatorManager(ctx context.Context, cluster *dev.Cluster) error {
	objs, err := dev.LoadKubernetesObjectsFromFile(
		"config/deploy/deployment.yaml.tpl")
	if err != nil {
		return fmt.Errorf("loading addon-operator-manager deployment.yaml.tpl: %w", err)
	}
	// Replace image
	addonOperatorDeployment := &appsv1.Deployment{}
	if err := cluster.Scheme.Convert(
		&objs[0], addonOperatorDeployment, nil); err != nil {
		return fmt.Errorf("converting to Deployment: %w", err)
	}
	addonOperatorManagerImage := os.Getenv("ADDON_OPERATOR_MANAGER_IMAGE")
	if len(addonOperatorManagerImage) == 0 {
		addonOperatorManagerImage = imageOrg + "/addon-operator-manager:" + version
	}
	addonOperatorDeployment.Spec.Template.Spec.Containers[0].Image = addonOperatorManagerImage
	// Deploy
	if err := cluster.CreateAndWaitFromFiles(ctx, []string{
		// TODO: replace with CreateAndWaitFromFolders when deployment.yaml is gone.
		"config/deploy/00-namespace.yaml",
		"config/deploy/addons.managed.openshift.io_addoninstances.yaml",
		"config/deploy/addons.managed.openshift.io_addonoperators.yaml",
		"config/deploy/addons.managed.openshift.io_addons.yaml",
		"config/deploy/rbac.yaml",
	}); err != nil {
		return fmt.Errorf("deploy addon-operator-manager dependencies: %w", err)
	}
	if err := cluster.CreateAndWaitForReadiness(ctx, addonOperatorDeployment); err != nil {
		return fmt.Errorf("deploy addon-operator-manager: %w", err)
	}
	return nil
}

// Addon Operator Webhook server from local files.
func deployAddonOperatorWebhook(ctx context.Context, cluster *dev.Cluster) error {
	objs, err := dev.LoadKubernetesObjectsFromFile(
		"config/deploy/webhook/deployment.yaml.tpl")
	if err != nil {
		return fmt.Errorf("loading addon-operator-webhook deployment.yaml.tpl: %w", err)
	}

	// Replace image
	addonOperatorWebhookDeployment := &appsv1.Deployment{}
	if err := cluster.Scheme.Convert(
		&objs[0], addonOperatorWebhookDeployment, nil); err != nil {
		return fmt.Errorf("converting to Deployment: %w", err)
	}
	addonOperatorWebhookImage := os.Getenv("ADDON_OPERATOR_WEBHOOK_IMAGE")
	if len(addonOperatorWebhookImage) == 0 {
		addonOperatorWebhookImage = imageOrg + "/addon-operator-webhook:" + version
	}
	addonOperatorWebhookDeployment.Spec.Template.Spec.Containers[0].Image = addonOperatorWebhookImage
	// Deploy
	if err := cluster.CreateAndWaitFromFiles(ctx, []string{
		// TODO: replace with CreateAndWaitFromFolders when deployment.yaml is gone.
		"config/deploy/webhook/00-tls-secret.yaml",
		"config/deploy/webhook/service.yaml",
		"config/deploy/webhook/validatingwebhookconfig.yaml",
	}); err != nil {
		return fmt.Errorf("deploy addon-operator-webhook dependencies: %w", err)
	}
	if err := cluster.CreateAndWaitForReadiness(ctx, addonOperatorWebhookDeployment); err != nil {
		return fmt.Errorf("deploy addon-operator-webhook: %w", err)
	}
	return nil
}

// Deploy all addon operator components to a cluster.
func deploy(
	ctx context.Context, cluster *dev.Cluster,
) error {
	if err := deployAPIMock(ctx, cluster); err != nil {
		return err
	}

	if err := deployAddonOperatorManager(ctx, cluster); err != nil {
		return err
	}

	if enableWebhooks, ok := os.LookupEnv("ENABLE_WEBHOOK"); ok &&
		enableWebhooks == "true" {
		if err := deployAddonOperatorWebhook(ctx, cluster); err != nil {
			return err
		}
	}
	return nil
}

// Setup a local cluster with all Addon Operator components running.
// Used to develop new test suites and for manual verification testing.
func (d Dev) Testing(ctx context.Context) error {
	mg.SerialDeps(
		Dev.init,
		Dev.loadAllImages,
	)
	if err := deploy(ctx, defaultDevEnvironment.Cluster); err != nil {
		return err
	}
	return nil
}

// Run integration test locally.
func (Dev) IntegrationTests() {
	mg.SerialDeps(
		Dev.Testing,
		Test.Integration,
	)
}

// Build and load container images into the dev environment.
func (Dev) loadAllImages() {
	mg.Deps(
		mg.F(Dev.imageLoad, "api-mock"),
		mg.F(Dev.imageLoad, "addon-operator-manager"),
		mg.F(Dev.imageLoad, "addon-operator-webhook"),
	)
}

// Load an image into the main kind cluster.
func (Dev) imageLoad(ctx context.Context, imageName string) error {
	mg.Deps(
		Dev.init,
		mg.F(Build.image, imageName),
	)

	imageTar := cacheDir + "/image/" + imageName + ".tar"
	if err := defaultDevEnvironment.LoadImageFromTar(ctx, imageTar); err != nil {
		return fmt.Errorf("load image: %w", err)
	}
	return nil
}

func (Dev) Teardown(ctx context.Context) error {
	if err := defaultDevEnvironment.Destroy(ctx); err != nil {
		return fmt.Errorf("destroying dev environment: %w", err)
	}
	return nil
}

// Generators
// ----------
type Generators mg.Namespace

// Prepare files for config/openshift
func (Generators) openshiftCITestBuild() error {
	if err := os.RemoveAll(workDir + "/config/openshift"); err != nil {
		return fmt.Errorf("clean up config/openshift: %w", err)
	}

	for _, dir := range []string{
		workDir + "/config/openshift/manifests",
		workDir + "/config/openshift/metadata",
	} {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return fmt.Errorf("create dir %q: %w", dir, err)
		}
	}

	for _, cmd := range [][]string{
		{"cp",
			"config/docker/addon-operator-bundle.Dockerfile",
			"config/openshift/addon-operator-bundle.Dockerfile"},
		{"cp",
			"config/olm/annotations.yaml",
			"config/openshift/metadata"},
		{"cp",
			"config/olm/addon-operator.csv.tpl.yaml",
			"config/openshift/manifests/addon-operator.csv.yaml"},
		{"sh", "-c",
			`tail -n"+3" "config/deploy/addons.managed.openshift.io_addons.yaml" > "config/openshift/manifests/addons.crd.yaml"`},
		{"sh", "-c",
			`tail -n"+3" "config/deploy/addons.managed.openshift.io_addonoperators.yaml" > "config/openshift/manifests/addonoperators.crd.yaml"`},
		{"sh", "-c",
			`tail -n"+3" "config/deploy/addons.managed.openshift.io_addoninstances.yaml" > "config/openshift/manifests/addoninstances.crd.yaml"`},
	} {
		if err := sh.Run(cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("running %q: %w", strings.Join(cmd, " "), err)
		}
	}

	return nil
}

func (Generators) code() error {
	mg.Deps(Dependency.controllerGen)

	manifestsCmd := exec.Command("controller-gen",
		"crd:crdVersions=v1", "rbac:roleName=addon-operator-manager",
		"paths=./...", "output:crd:artifacts:config=../config/deploy")
	manifestsCmd.Dir = workDir + "/apis"
	log.Println("exec:", strings.Join(manifestsCmd.Args, " "))
	if err := manifestsCmd.Run(); err != nil {
		return fmt.Errorf("generating kubernetes manifests: %w", err)
	}

	// code gen
	codeCmd := exec.Command("controller-gen", "object", "paths=./...")
	codeCmd.Dir = workDir + "/apis"
	log.Println("exec:", strings.Join(codeCmd.Args, " "))
	if err := codeCmd.Run(); err != nil {
		return fmt.Errorf("generating deep copy methods: %w", err)
	}

	// patching generated code to stay go 1.16 output compliant
	// https://golang.org/doc/go1.17#gofmt
	// @TODO: remove this when we move to go 1.17"
	// otherwise our ci will fail because of changed files"
	// this removes the line '//go:build !ignore_autogenerated'"
	if err := sh.Run("find", ".", "-name", "zz_generated.deepcopy.go", "-exec", "sed", "-i", `/\/\/go:build !ignore_autogenerated/d`, "{}", ";"); err != nil {
		return fmt.Errorf("removing go:build annotation: %w", err)
	}

	return nil
}

func (Generators) docs() error {
	mg.Deps(Build.Docgen)

	return sh.Run("./hack/docgen.sh")
}

// Dependencies
// ------------

type Dependency mg.Namespace

func (d Dependency) kind() error {
	return depsDir.GoInstall("kind",
		"sigs.k8s.io/kind", kindVersion)
}

func (d Dependency) controllerGen() error {
	return depsDir.GoInstall("controller-gen",
		"sigs.k8s.io/controller-tools/cmd/controller-gen", controllerGenVersion)
}

func (d Dependency) yq() error {
	return depsDir.GoInstall("yq",
		"github.com/mikefarah/yq/v4", yqVersion)
}

func (d Dependency) Goimports() error {
	return depsDir.GoInstall("go-imports",
		"golang.org/x/tools/cmd/goimports", goimportsVersion)
}

func (d Dependency) GolangciLint() error {
	return depsDir.GoInstall("golangci-lint",
		"github.com/golangci/golangci-lint/cmd/golangci-lint", golangciLintVersion)
}

func (d Dependency) helm() error {
	return depsDir.GoInstall("helm", "helm.sh/helm/v3/cmd/helm", helmVersion)
}

func (d Dependency) opm() error {
	needsRebuild, err := depsDir.NeedsRebuild("opm", opmVersion)
	if err != nil {
		return err
	}
	if !needsRebuild {
		return nil
	}

	// Tempdir
	tempDir, err := os.MkdirTemp(".cache", "")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Download
	tempOPMBin := path.Join(tempDir, "opm")
	if err := sh.Run(
		"curl", "-L", "--fail",
		"-o", tempOPMBin,
		fmt.Sprintf(
			"https://github.com/protocolbuffers/protobuf/releases/download/%s/linux-amd64-opm",
			opmVersion,
		),
	); err != nil {
		return fmt.Errorf("downloading protoc: %w", err)
	}

	if err := os.Chmod(tempOPMBin, 0755); err != nil {
		return fmt.Errorf("make opm executable: %w", err)
	}

	// Move
	if err := os.Rename(tempOPMBin, path.Join(depsDir.Bin(), "opm")); err != nil {
		return fmt.Errorf("move protoc: %w", err)
	}
	return nil
}
