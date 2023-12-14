//go:build mage
// +build mage

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/mt-sre/client"
	"github.com/mt-sre/devkube/dev"
	imageparser "github.com/novln/docker-parser"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/yaml"
)

type Build mg.Namespace

// Build Tags
var (
	branch        string
	shortCommitID string
	version       string
	buildDate     string
	ldFlags       string
	imageOrg      string
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

// Builds the docgen internal tool
func (Build) Docgen() {
	mg.Deps(mg.F(Build.cmd, "docgen", "", ""))
}

// Builds binaries from /cmd directory.
func (Build) cmd(cmd, goos, goarch string) error {
	mg.Deps(Build.init)

	env := map[string]string{
		"GOFLAGS": "",
		"LDFLAGS": ldFlags,
	}

	_, cgoOK := os.LookupEnv("CGO_ENABLED")
	if !cgoOK {
		env["CGO_ENABLED"] = "0"
	}

	bin := path.Join("bin", cmd)

	if len(goos) != 0 && len(goarch) != 0 {
		// change bin path to point to a subdirectory when cross compiling
		bin = path.Join("bin", goos+"_"+goarch, cmd)
		env["GOOS"] = goos
		env["GOARCH"] = goarch
	}

	if cmd == "addon-operator-manager" {
		if err := sh.RunWithV(
			env,
			"go", "build", "-v", "-o", bin, ".",
		); err != nil {
			return fmt.Errorf("compiling addon-operator-manager: %v", err)
		}
	} else {
		if err := sh.RunWithV(
			env,
			"go", "build", "-v", "-o", bin, "./cmd/"+cmd,
		); err != nil {
			return fmt.Errorf("compiling cmd/%s: %w", cmd, err)
		}
	}
	return nil
}

// Default build target for CI/CD to build binaries
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

func (b Build) ImageBuild(cmd string) error {
	mg.SerialDeps(setupContainerRuntime, b.init)

	// clean/prepare cache directory
	imageCacheDir := path.Join(cacheDir, "image", cmd)
	if err := cleanImageCache(imageCacheDir); err != nil {
		return fmt.Errorf("cleaning cache: %w", err)
	}

	switch cmd {
	case "addon-operator-index":
		return b.buildOLMIndexImage()

	case "addon-operator-bundle":
		return b.buildOLMBundleImage(imageCacheDir)

	case "addon-operator-package":
		return b.buildPackageOperatorImage(imageCacheDir)

	default:
		deps := []interface{}{
			mg.F(Build.cmd, cmd, "linux", "amd64"),
			mg.F(populateCmdCache, imageCacheDir, cmd),
		}
		imageBuildInfo := newImageBuildInfo(cmd, imageCacheDir)
		return dev.BuildImage(imageBuildInfo, deps)
	}
}

func (Build) BuildAndPushPackage() {
	mg.Deps(
		mg.F(Build.ImageBuild, "addon-operator-package"),
		mg.F(Build.imagePushOnce, "addon-operator-package"),
	)
}

func newImageBuildInfo(imageName, imageCacheDir string) *dev.ImageBuildInfo {
	imageTag := imageURL(imageName)
	return &dev.ImageBuildInfo{
		ImageTag:      imageTag,
		CacheDir:      imageCacheDir,
		ContainerFile: "",
		ContextDir:    imageCacheDir,
		Runtime:       containerRuntime,
	}
}

func (b Build) buildOLMIndexImage() error {
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

func populateOLMBundleCache(imageCacheDir string) error {
	manifestsDir := path.Join(imageCacheDir, "manifests")
	metadataDir := path.Join(imageCacheDir, "metadata")
	for _, command := range [][]string{
		{"mkdir", "-p", manifestsDir},
		{"mkdir", "-p", metadataDir},

		// Copy files for build environment
		{"cp", "-a",
			"deploy-extras/docker/addon-operator-bundle.Dockerfile",
			imageCacheDir + "/Dockerfile"},

		{"cp", "-a", "deploy-extras/olm/addon-operator.csv.yaml", manifestsDir},
		{"cp", "-a", "deploy/45_metrics-service.yaml", manifestsDir},
		{"cp", "-a", "deploy/50_servicemonitor.yaml", manifestsDir},
		{"cp", "-a", "deploy/35_prometheus-role.yaml", manifestsDir},
		{"cp", "-a", "deploy/40_prometheus-rolebinding.yaml", manifestsDir},
		{"cp", "-a", "deploy-extras/olm/annotations.yaml", metadataDir},
		{"cp", "-a", "deploy/55_trusted_ca_bundle_configmap.yaml", manifestsDir},
		// copy CRDs
		// The first few lines of the CRD file need to be removed:
		// https://github.com/operator-framework/operator-registry/issues/222
		{"bash", "-c", "tail -n+3 " +
			"deploy/crds/addons.managed.openshift.io_addons.yaml " +
			"> " + path.Join(manifestsDir, "addons.yaml")},
		{"bash", "-c", "tail -n+3 " +
			"deploy/crds/addons.managed.openshift.io_addonoperators.yaml " +
			"> " + path.Join(manifestsDir, "addonoperators.yaml")},
		{"bash", "-c", "tail -n+3 " +
			"deploy/crds/addons.managed.openshift.io_addoninstances.yaml " +
			"> " + path.Join(manifestsDir, "addoninstances.yaml")},
	} {
		if err := sh.RunV(command[0], command[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func (b Build) buildOLMBundleImage(imageCacheDir string) error {
	deps := []interface{}{
		mg.F(Build.init),
		mg.F(Build.TemplateAddonOperatorCSV),
		mg.F(populateOLMBundleCache, imageCacheDir),
	}
	buildInfo := newImageBuildInfo("addon-operator-bundle", imageCacheDir)
	return dev.BuildImage(buildInfo, deps)
}

func populatePkgCache(imageCacheDir string) error {
	manifestsDir := path.Join(imageCacheDir, "manifests")
	for _, command := range [][]string{
		{"mkdir", "-p", manifestsDir},
		{"bash", "-c", "cp hack/hypershift/package/*.yaml " + manifestsDir},
		{"cp", "hack/hypershift/package/addon-operator-package.Containerfile", manifestsDir},
	} {
		if err := sh.RunV(command[0], command[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func newPackageBuildInfo(imageCacheDir string) *dev.PackageBuildInfo {
	imageTag := imageURL("addon-operator-package")
	manifestsDir := path.Join(imageCacheDir, "manifests")

	return &dev.PackageBuildInfo{
		ImageTag:   imageTag,
		CacheDir:   imageCacheDir,
		SourcePath: manifestsDir,
		OutputPath: manifestsDir + ".tar",
		Runtime:    containerRuntime,
	}
}

func (b Build) buildPackageOperatorImage(imageCacheDir string) error {
	mg.Deps(
		Build.init,
	)

	deployment := &appsv1.Deployment{}
	err := loadAndUnmarshalIntoObject("deploy-extras/package/hcp/addon-operator-template.yaml", deployment)
	if err != nil {
		return fmt.Errorf("loading addon-operator-template.yaml: %w", err)
	}

	for i := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[i]

		if container.Name == "manager" {

			container.Image = getImageName("addon-operator-manager")
		}
	}

	depBytes, err := yaml.Marshal(deployment)
	if err != nil {
		return err
	}
	if err := os.WriteFile("hack/hypershift/addon-operator.yaml",
		depBytes, os.ModePerm); err != nil {
		return err
	}

	deps := []interface{}{
		Dependency.PkoCli,
		mg.F(populatePkgCache, imageCacheDir),
	}
	buildInfo := newPackageBuildInfo(imageCacheDir)
	return dev.BuildPackage(buildInfo, deps)
}

func (b Build) TemplateAddonOperatorCSV() error {
	// convert unstructured.Unstructured to CSV
	csvTemplate, err := os.ReadFile(path.Join(workDir, "deploy-extras/olm/addon-operator.csv.tpl.yaml"))
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
	if err := os.WriteFile("deploy-extras/olm/addon-operator.csv.yaml",
		csvBytes, os.ModePerm); err != nil {
		return err
	}

	return nil
}

func newImagePushInfo(imageName string) *dev.ImagePushInfo {
	imageTag := imageURL(imageName)
	return &dev.ImagePushInfo{
		ImageTag:   imageTag,
		CacheDir:   cacheDir,
		Runtime:    containerRuntime,
		DigestFile: "",
	}
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
	mg.SerialDeps(setupContainerRuntime, Build.init)

	pushInfo := newImagePushInfo(imageName)
	buildImageDep := mg.F(Build.ImageBuild, imageName)

	return dev.PushImage(pushInfo, buildImageDep)
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

func cleanImageCache(imageCacheDir string) error {
	if err := os.RemoveAll(imageCacheDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting image cache dir: %w", err)
	}
	if err := os.Remove(imageCacheDir + ".tar"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting image tar: %w", err)
	}
	if err := os.MkdirAll(imageCacheDir, os.ModePerm); err != nil {
		return fmt.Errorf("create image cache dir: %w", err)
	}
	return nil
}

func populateCmdCache(imageCacheDir, cmd string) error {
	commands := [][]string{
		{"cp", "-a", "bin/linux_amd64/" + cmd, imageCacheDir + "/" + cmd},
		{"cp", "-a", "deploy-extras/docker/" + cmd + ".Dockerfile", imageCacheDir + "/Dockerfile"},
	}
	for _, command := range commands {
		if err := sh.Run(command[0], command[1:]...); err != nil {
			return fmt.Errorf("running %q: %w", strings.Join(command, " "), err)
		}
	}
	return nil
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
