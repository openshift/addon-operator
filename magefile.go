//go:build mage
// +build mage

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/mt-sre/client"
	"github.com/mt-sre/devkube/dev"
	"github.com/mt-sre/devkube/magedeps"
	imageparser "github.com/novln/docker-parser"
	olmversion "github.com/operator-framework/api/pkg/lib/version"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	aoapisv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"

	"github.com/openshift/addon-operator/internal/featuretoggle"
)

const (
	module          = "github.com/openshift/addon-operator"
	defaultImageOrg = "quay.io/app-sre"
)

// Directories
var (
	// Working directory of the project.
	workDir string
	// Dependency directory.
	depsDir  magedeps.DependencyDirectory
	cacheDir string

	logger           logr.Logger
	containerRuntime string
)

func init() {
	var err error
	// Directories
	workDir, err = os.Getwd()
	if err != nil {
		panic(fmt.Errorf("getting work dir: %w", err))
	}
	cacheDir = path.Join(workDir + "/" + ".cache")
	depsDir = magedeps.DependencyDirectory(path.Join(workDir, ".deps"))
	os.Setenv("PATH", depsDir.Bin()+":"+os.Getenv("PATH"))

	logger = stdr.New(nil)
}

// dependency for all targets requiring a container runtime
func setupContainerRuntime() {
	containerRuntime = os.Getenv("CONTAINER_RUNTIME")
	if len(containerRuntime) == 0 || containerRuntime == "auto" {
		cr, err := dev.DetectContainerRuntime()
		if err != nil {
			panic(err)
		}
		containerRuntime = string(cr)
		logger.Info("detected container-runtime", "container-runtime", containerRuntime)
	}
}

// Prepare a new release of the Addon Operator.
func Prepare_Release() error {
	versionBytes, err := os.ReadFile(path.Join(workDir, "VERSION"))
	if err != nil {
		return fmt.Errorf("reading VERSION file: %w", err)
	}

	version = strings.TrimSpace(strings.TrimLeft(string(versionBytes), "v"))
	semverVersion, err := semver.New(version)
	if err != nil {
		return fmt.Errorf("parse semver: %w", err)
	}

	// read CSV
	csvTemplate, err := os.ReadFile(path.Join(workDir, "config/olm/addon-operator.csv.tpl.yaml"))
	if err != nil {
		return fmt.Errorf("reading CSV template: %w", err)
	}

	var csv operatorsv1alpha1.ClusterServiceVersion
	if err := yaml.Unmarshal(csvTemplate, &csv); err != nil {
		return err
	}

	// Update for new release
	csv.Annotations["olm.skipRange"] = ">=0.0.1 <" + version
	csv.Name = "addon-operator.v" + version
	csv.Spec.Version = olmversion.OperatorVersion{Version: *semverVersion}

	// write updated template
	csvBytes, err := yaml.Marshal(csv)
	if err != nil {
		return err
	}
	if err := os.WriteFile("config/olm/addon-operator.csv.tpl.yaml",
		csvBytes, os.ModePerm); err != nil {
		return err
	}

	// run generators to re-template config/openshift/manifests/*
	if err := sh.RunV("make", "openshift-ci-test-build"); err != nil {
		return fmt.Errorf("rebuilding config/openshift/: %w", err)
	}
	return nil
}

// Building
// --------
type Build mg.Namespace

// Build Tags
var (
	branch        string
	shortCommitID string
	version       string
	buildDate     string

	ldFlags string

	imageOrg string
)

// init build variables
func (Build) init() error {
	// Build flags
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
	ldFlags = fmt.Sprintf(`-X %s/internal/version.Version=%s`+
		`-X %s/internal/version.Branch=%s`+
		`-X %s/internal/version.Commit=%s`+
		`-X %s/internal/version.BuildDate=%s`,
		module, version,
		module, branch,
		module, shortCommitID,
		module, buildDate,
	)

	imageOrg = os.Getenv("IMAGE_ORG")
	if len(imageOrg) == 0 {
		imageOrg = defaultImageOrg
	}

	return nil
}

// Builds binaries from /cmd directory.
func (Build) cmd(cmd, goos, goarch string) error {
	mg.Deps(Build.init)

	env := map[string]string{
		"GOFLAGS":     "",
		"CGO_ENABLED": "0",
		"LDFLAGS":     ldFlags,
	}

	bin := path.Join("bin", cmd)
	if len(goos) != 0 && len(goarch) != 0 {
		// change bin path to point to a subdirectory when cross compiling
		bin = path.Join("bin", goos+"_"+goarch, cmd)
		env["GOOS"] = goos
		env["GOARCH"] = goarch
	}

	if err := sh.RunWithV(
		env,
		"go", "build", "-v", "-o", bin, "./cmd/"+cmd,
	); err != nil {
		return fmt.Errorf("compiling cmd/%s: %w", cmd, err)
	}
	return nil
}

// Default build target for CI/CD
func (Build) All() {
	mg.Deps(
		mg.F(Build.cmd, "addon-operator-manager", "linux", "amd64"),
		mg.F(Build.cmd, "addon-operator-webhook", "linux", "amd64"),
		mg.F(Build.cmd, "api-mock", "linux", "amd64"),
		mg.F(Build.cmd, "mage", "", ""),
	)
}

func (Build) BuildImages() {
	mg.Deps(
		mg.F(Build.ImageBuild, "addon-operator-manager"),
		mg.F(Build.ImageBuild, "addon-operator-webhook"),
		mg.F(Build.ImageBuild, "api-mock"),
		mg.F(Build.ImageBuild, "addon-operator-index"), // also pushes bundle
		mg.F(Build.ImageBuild, "addon-operator-package"),
	)
}

func (Build) PushImages() {
	mg.Deps(
		mg.F(Build.imagePush, "addon-operator-manager"),
		mg.F(Build.imagePush, "addon-operator-webhook"),
		mg.F(Build.imagePush, "addon-operator-index"), // also pushes bundle
		mg.F(Build.imagePush, "addon-operator-package"),
	)
}

func (Build) PushImagesOnce() {
	mg.Deps(
		mg.F(Build.imagePushOnce, "addon-operator-manager"),
		mg.F(Build.imagePushOnce, "addon-operator-webhook"),
		mg.F(Build.imagePushOnce, "addon-operator-index"), // also pushes bundle
		mg.F(Build.imagePushOnce, "addon-operator-package"),
	)
}

// Builds the docgen internal tool
func (Build) Docgen() {
	mg.Deps(mg.F(Build.cmd, "docgen", "", ""))
}

func (b Build) ImageBuild(cmd string) error {
	mg.SerialDeps(setupContainerRuntime)

	// clean/prepare cache directory
	imageCacheDir := path.Join(cacheDir, "image", cmd)
	if err := os.RemoveAll(imageCacheDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting image cache dir: %w", err)
	}
	if err := os.Remove(imageCacheDir + ".tar"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting image tar: %w", err)
	}
	if err := os.MkdirAll(imageCacheDir, os.ModePerm); err != nil {
		return fmt.Errorf("create image cache dir: %w", err)
	}

	switch cmd {
	case "addon-operator-index":
		return b.buildOLMIndexImage(imageCacheDir)

	case "addon-operator-bundle":
		return b.buildOLMBundleImage(imageCacheDir)

	case "addon-operator-package":
		return b.buildPackageOperatorImage(imageCacheDir)

	default:
		mg.Deps(
			mg.F(Build.cmd, cmd, "linux", "amd64"),
		)
		return b.buildGenericImage(cmd, imageCacheDir)
	}
}

// generic image build function, when the image just relies on
// a static binary build from cmd/*
func (Build) buildGenericImage(cmd, imageCacheDir string) error {
	imageTag := imageURL(cmd)
	for _, command := range [][]string{
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
		if err := sh.Run(command[0], command[1:]...); err != nil {
			return fmt.Errorf("running %q: %w", strings.Join(command, " "), err)
		}
	}
	return nil
}

func (b Build) buildOLMIndexImage(imageCacheDir string) error {
	mg.Deps(
		Dependency.Opm,
		mg.F(Build.imagePush, "addon-operator-bundle"),
	)

	if err := sh.RunV("opm", "index", "add",
		"--container-tool", containerRuntime,
		"--bundles", imageURL("addon-operator-bundle"),
		"--tag", imageURL("addon-operator-index")); err != nil {
		return fmt.Errorf("running opm: %w", err)
	}
	return nil
}

func (b Build) buildOLMBundleImage(imageCacheDir string) error {
	mg.Deps(
		Build.init,
		Build.TemplateAddonOperatorCSV,
	)

	imageTag := imageURL("addon-operator-bundle")
	manifestsDir := path.Join(imageCacheDir, "manifests")
	metadataDir := path.Join(imageCacheDir, "metadata")
	for _, command := range [][]string{
		{"mkdir", "-p", manifestsDir},
		{"mkdir", "-p", metadataDir},

		// Copy files for build environment
		{"cp", "-a",
			"config/docker/addon-operator-bundle.Dockerfile",
			imageCacheDir + "/Dockerfile"},

		{"cp", "-a", "config/olm/addon-operator.csv.yaml", manifestsDir},
		{"cp", "-a", "config/olm/metrics.service.yaml", manifestsDir},
		{"cp", "-a", "config/olm/addon-operator-servicemonitor.yaml", manifestsDir},
		{"cp", "-a", "config/olm/prometheus-role.yaml", manifestsDir},
		{"cp", "-a", "config/olm/prometheus-rb.yaml", manifestsDir},
		{"cp", "-a", "config/olm/annotations.yaml", metadataDir},

		// copy CRDs
		// The first few lines of the CRD file need to be removed:
		// https://github.com/operator-framework/operator-registry/issues/222
		{"bash", "-c", "tail -n+3 " +
			"config/deploy/addons.managed.openshift.io_addons.yaml " +
			"> " + path.Join(manifestsDir, "addons.yaml")},
		{"bash", "-c", "tail -n+3 " +
			"config/deploy/addons.managed.openshift.io_addonoperators.yaml " +
			"> " + path.Join(manifestsDir, "addonoperators.yaml")},
		{"bash", "-c", "tail -n+3 " +
			"config/deploy/addons.managed.openshift.io_addoninstances.yaml " +
			"> " + path.Join(manifestsDir, "addoninstances.yaml")},

		// Build image!
		{containerRuntime, "build", "-t", imageTag, imageCacheDir},
		{containerRuntime, "image", "save",
			"-o", imageCacheDir + ".tar", imageTag},
	} {
		if err := sh.RunV(command[0], command[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func (b Build) buildPackageOperatorImage(imageCacheDir string) error {
	mg.Deps(
		Build.init,
	)

	deployment := &appsv1.Deployment{}
	err := loadAndUnmarshalIntoObject("config/package/hcp/addon-operator.yaml.tpl", deployment)
	if err != nil {
		return fmt.Errorf("loading addon-operator.yaml.tpl: %w", err)
	}

	// Replace image
	patchDeployment(deployment, "addon-operator-manager", "manager")

	depBytes, err := yaml.Marshal(deployment)
	if err != nil {
		return err
	}
	if err := os.WriteFile("config/package/hcp/addon-operator.yaml",
		depBytes, os.ModePerm); err != nil {
		return err
	}

	imageTag := imageURL("addon-operator-package")

	manifestsDir := path.Join(imageCacheDir, "manifests")

	for _, command := range [][]string{
		{"mkdir", "-p", manifestsDir},
		{"bash", "-c", "cp config/package/hc/*.yaml " + manifestsDir},
		{"cp", "config/package/hcp/addon-operator.yaml", manifestsDir},
		{"cp", "config/package/manifest.yaml", manifestsDir},

		{"cp", "config/package/addon-operator-package.Containerfile", manifestsDir},
		{containerRuntime, "build", "-t", imageTag, "-f", manifestsDir + "/addon-operator-package.Containerfile"},
		{containerRuntime, "image", "save", "-o", imageCacheDir + ".tar", imageTag},
	} {
		if err := sh.RunV(command[0], command[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func (b Build) TemplateAddonOperatorCSV() error {
	// convert unstructured.Unstructured to CSV
	csvTemplate, err := os.ReadFile(path.Join(workDir, "config/olm/addon-operator.csv.tpl.yaml"))
	if err != nil {
		return fmt.Errorf("reading CSV template: %w", err)
	}

	var csv operatorsv1alpha1.ClusterServiceVersion
	if err := yaml.Unmarshal(csvTemplate, &csv); err != nil {
		return err
	}

	// replace images
	for i := range csv.Spec.
		InstallStrategy.StrategySpec.DeploymentSpecs {
		deploy := &csv.Spec.
			InstallStrategy.StrategySpec.DeploymentSpecs[i]

		switch deploy.Name {
		case "addon-operator-manager":
			for i := range deploy.Spec.
				Template.Spec.Containers {
				container := &deploy.Spec.Template.Spec.Containers[i]
				switch container.Name {
				case "manager":
					container.Image = imageURL("addon-operator-manager")
				}
			}

		case "addon-operator-webhooks":
			for i := range deploy.Spec.
				Template.Spec.Containers {
				container := &deploy.Spec.Template.Spec.Containers[i]
				switch container.Name {
				case "webhook":
					container.Image = imageURL("addon-operator-webhook")
				}
			}
		}
	}
	csv.Annotations["containerImage"] = imageURL("addon-operator-manager")

	// write
	csvBytes, err := yaml.Marshal(csv)
	if err != nil {
		return err
	}
	if err := os.WriteFile("config/olm/addon-operator.csv.yaml",
		csvBytes, os.ModePerm); err != nil {
		return err
	}

	return nil
}

func (b Build) imagePushOnce(imageName string) error {
	mg.SerialDeps(
		Build.init,
	)

	ok, err := b.imageExists(context.Background(), imageName)
	if err != nil {
		return fmt.Errorf("checking if image %q exists: %w", imageName, err)
	}

	if ok {
		fmt.Fprintf(os.Stdout, "skipping image %q since it is already up-to-date\n", imageName)

		return nil
	}

	return b.imagePush(imageName)
}

func (Build) imagePush(imageName string) error {
	mg.SerialDeps(
		mg.F(Build.ImageBuild, imageName),
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

	if err := sh.Run(containerRuntime, "push", imageURL(imageName)); err != nil {
		return fmt.Errorf("pushing image: %w", err)
	}

	return nil
}

func (Build) imageExists(ctx context.Context, name string) (bool, error) {
	ref, err := imageparser.Parse(imageURL(name))
	if err != nil {
		return false, fmt.Errorf("parsing image reference: %w", err)
	}

	url := url.URL{
		Scheme: "https",
		Host:   ref.Registry(),
		Path:   path.Join("v2", ref.ShortName(), "manifests", ref.Tag()),
	}

	c := client.NewClient()
	res, err := c.Head(ctx, url.String())
	if err != nil {
		return false, fmt.Errorf("sending HTTP request: %w", err)
	}

	defer res.Body.Close()

	return res.StatusCode == http.StatusOK, nil
}

func imageURL(name string) string {
	// Build.init must be run before this function to set `imageOrg` and `version` variables
	envvar := strings.ReplaceAll(strings.ToUpper(name), "-", "_") + "_IMAGE"
	if url := os.Getenv(envvar); len(url) != 0 {
		return url
	}
	if len(version) == 0 {
		panic("empty version, refusing to return container image URL")
	}
	return imageOrg + "/" + name + ":" + version
}

// Code Generators
// ---------------
type Generate mg.Namespace

func (Generate) All() {
	mg.Deps(
		Generate.code,
		Generate.docs,
	)
}

func (Generate) code() error {
	mg.Deps(Dependency.ControllerGen)

	manifestsCmd := exec.Command("controller-gen",
		"crd:crdVersions=v1", "rbac:roleName=addon-operator-manager",
		"paths=./...", "output:crd:artifacts:config=../config/deploy")
	manifestsCmd.Dir = workDir + "/apis"
	if err := manifestsCmd.Run(); err != nil {
		return fmt.Errorf("generating kubernetes manifests: %w", err)
	}

	// code gen
	codeCmd := exec.Command("controller-gen", "object", "paths=./...")
	codeCmd.Dir = workDir + "/apis"
	if err := codeCmd.Run(); err != nil {
		return fmt.Errorf("generating deep copy methods: %w", err)
	}

	// patching generated code to stay go 1.16 output compliant
	// https://golang.org/doc/go1.17#gofmt
	// @TODO: remove this when we move to go 1.17"
	// otherwise our ci will fail because of changed files"
	// this removes the line '//go:build !ignore_autogenerated'"
	findArgs := []string{".", "-name", "zz_generated.deepcopy.go", "-exec",
		"sed", "-i", `/\/\/go:build !ignore_autogenerated/d`, "{}", ";"}

	// The `-i` flag works a bit differently on MacOS (I don't know why.)
	// See - https://stackoverflow.com/a/19457213
	if goruntime.GOOS == "darwin" && !gnuSed() {
		findArgs = []string{".", "-name", "zz_generated.deepcopy.go", "-exec",
			"sed", "-i", "", "-e", `/\/\/go:build !ignore_autogenerated/d`, "{}", ";"}
	}
	if err := sh.Run("find", findArgs...); err != nil {
		return fmt.Errorf("removing go:build annotation: %w", err)
	}

	return nil
}

func (Generate) docs() error {
	mg.Deps(Build.Docgen)

	return sh.Run("./hack/docgen.sh")
}

// Testing and Linting
// -------------------
type Test mg.Namespace

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

// Runs unittests.
func (Test) Unit() error {
	return sh.RunWithV(map[string]string{
		// needed to enable race detector -race
		"CGO_ENABLED": "1",
	}, "go", "test", "-cover", "-v", "-race", "./internal/...", "./cmd/...", "./pkg/...")
}

func (Test) Integration() error {
	return sh.Run("go", "test", "-v",
		"-count=1", // will force a new run, instead of using the cache
		"-timeout=25m", "./integration/...")
}

// Target to prepare the CI-CD environment before installing the operator.
func (t Test) IntegrationCIPrepare(ctx context.Context) error {
	cluster, err := dev.NewCluster(path.Join(cacheDir, "ci"),
		dev.WithKubeconfigPath(os.Getenv("KUBECONFIG")))
	if err != nil {
		return fmt.Errorf("creating cluster client: %w", err)
	}

	ctx = logr.NewContext(ctx, logger)
	return labelNodesWithInfraRole(ctx, cluster)
}

// Target to inject the addon status reporting environment variable
// to the addon operator CSV present in the cluster.
func (t Test) IntegrationCIInjectEnvVariable(ctx context.Context) error {
	cluster, err := dev.NewCluster(path.Join(cacheDir, "ci"),
		dev.WithKubeconfigPath(os.Getenv("KUBECONFIG")), dev.WithSchemeBuilder(operatorsv1alpha1.SchemeBuilder))
	if err != nil {
		return fmt.Errorf("creating cluster client: %w", err)
	}

	ctx = dev.ContextWithLogger(ctx, logger)
	if err = injectStatusReportingEnvironmentVariable(ctx, cluster); err != nil {
		return fmt.Errorf("Inject ENV into CSV failed: %w", err)
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
	cluster, err := dev.NewCluster(path.Join(cacheDir, "ci"),
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

	return t.Integration()
}

func (Test) IntegrationShort() error {
	return sh.Run("go", "test", "-v",
		"-count=1", // will force a new run, instead of using the cache
		"-short",
		"-timeout=20m", "./integration/...")
}

// Dependencies
// ------------

// Dependency Versions
const (
	controllerGenVersion = "0.6.2"
	kindVersion          = "0.11.1"
	yqVersion            = "4.12.0"
	goimportsVersion     = "0.1.5"
	golangciLintVersion  = "1.46.2"
	olmVersion           = "0.20.0"
	opmVersion           = "1.24.0"
	helmVersion          = "3.7.2"
)

type Dependency mg.Namespace

func (d Dependency) All() {
	mg.Deps(
		Dependency.Kind,
		Dependency.ControllerGen,
		Dependency.YQ,
		Dependency.Goimports,
		Dependency.GolangciLint,
		Dependency.Helm,
		Dependency.Opm,
	)
}

// Ensure Kind dependency - Kubernetes in Docker (or Podman)
func (d Dependency) Kind() error {
	return depsDir.GoInstall("kind",
		"sigs.k8s.io/kind", kindVersion)
}

// Ensure controller-gen - kubebuilder code and manifest generator.
func (d Dependency) ControllerGen() error {
	return depsDir.GoInstall("controller-gen",
		"sigs.k8s.io/controller-tools/cmd/controller-gen", controllerGenVersion)
}

// Ensure yq - jq but for Yaml, written in Go.
func (d Dependency) YQ() error {
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

func (d Dependency) Helm() error {
	return depsDir.GoInstall("helm", "helm.sh/helm/v3/cmd/helm", helmVersion)
}

func (d Dependency) Opm() error {
	// TODO: move this into devkube library, to ensure the depsDir is present, even if you just call "NeedsRebuild"
	if err := os.MkdirAll(depsDir.Bin(), os.ModePerm); err != nil {
		return fmt.Errorf("create dependency dir: %w", err)
	}

	needsRebuild, err := depsDir.NeedsRebuild("opm", opmVersion)
	if err != nil {
		return err
	}
	if !needsRebuild {
		return nil
	}

	// Tempdir
	tempDir, err := os.MkdirTemp(cacheDir, "")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Download
	tempOPMBin := path.Join(tempDir, "opm")
	if err := sh.RunV(
		"curl", "-L", "--fail",
		"-o", tempOPMBin,
		fmt.Sprintf(
			"https://github.com/operator-framework/operator-registry/releases/download/v%s/linux-amd64-opm",
			opmVersion,
		),
	); err != nil {
		return fmt.Errorf("downloading opm: %w", err)
	}

	if err := os.Chmod(tempOPMBin, 0755); err != nil {
		return fmt.Errorf("make opm executable: %w", err)
	}

	// Move
	if err := os.Rename(tempOPMBin, path.Join(depsDir.Bin(), "opm")); err != nil {
		return fmt.Errorf("move opm: %w", err)
	}
	return nil
}

// Development
// --------
type Dev mg.Namespace

var (
	devEnvironment *dev.Environment
)

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

	mg.SerialDeps(Test.Integration)
	return nil
}

func (d Dev) LoadImage(ctx context.Context, image string) error {
	mg.Deps(
		mg.F(Build.ImageBuild, image),
	)

	imageTar := path.Join(cacheDir, "image", image+".tar")
	if err := devEnvironment.LoadImageFromTar(ctx, imageTar); err != nil {
		return fmt.Errorf("load image from tar: %w", err)
	}
	return nil
}

// Deploy the Addon Operator, Mock API Server and Addon Operator webhooks (if env ENABLE_WEBHOOK=true) is set.
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
	if err := d.deployAPIMock(ctx, cluster); err != nil {
		return err
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
		if featTog.IsEnabledOnTestEnv() {
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
		if featTog.IsEnabledOnTestEnv() {
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
		if featTog.IsEnabledOnTestEnv() {
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

	availableFeatureToggles := []featuretoggle.FeatureToggleHandler{
		featuretoggle.MonitoringStackFeatureToggle{
			SchemeToUpdate: cluster.Scheme,
		},
	}

	for _, featTog := range availableFeatureToggles {
		// feature toggles enabled/disabled at the level of openshift/release in the form of multiple jobs
		if featTog.IsEnabled() {
			if err := featTog.EnableOnAddonOperatorDeployment(addonOperatorDeployment); err != nil {
				return fmt.Errorf("failed to set the feature toggle '%s': %w", featTog.Name(), err)
			}
		}
	}

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

func (d Dev) init() error {
	mg.SerialDeps(
		setupContainerRuntime,
		Dependency.Kind,
	)

	devEnvironment = dev.NewEnvironment(
		"addon-operator-dev",
		path.Join(cacheDir, "dev-env"),
		dev.WithClusterOptions([]dev.ClusterOption{
			dev.WithWaitOptions([]dev.WaitOption{
				dev.WithTimeout(2 * time.Minute),
			}),
			dev.WithSchemeBuilder(aoapisv1alpha1.SchemeBuilder),
		}),
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
		})
	return nil
}

func gnuSed() bool {
	cmd := exec.Command("sed", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("warning: sed --version returned error. this could mean that 'sed' is not 'gnu sed':", string(output), err)
		return false
	}
	return strings.Contains(string(output), "GNU")
}
