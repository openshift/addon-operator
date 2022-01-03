//go:build mage
// +build mage

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/magefile/mage/target"
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
)

// Directories
var (
	// Working directory of the project.
	workDir string
	// Cache directory for temporary build files.
	cacheDir string
	// Dependency directory.
	depsDir string
)

// Build Tags
var (
	branch        string
	shortCommitID string
	version       string
	buildDate     string

	ldFlags string
)

func init() {
	var err error
	// Directories
	workDir, err = os.Getwd()
	if err != nil {
		panic(fmt.Errorf("getting work dir: %w", err))
	}

	depsDir = workDir + "/.deps"
	cacheDir = workDir + "/.cache"

	// Path
	os.Setenv("PATH", depsDir+"/bin:"+os.Getenv("PATH"))

	// Build Tags

	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchBytes, err := branchCmd.Output()
	if err != nil {
		panic(fmt.Errorf("getting git branch: %w", err))
	}
	branch = string(branchBytes)

	shortCommitIDCmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	shortCommitIDBytes, err := shortCommitIDCmd.Output()
	if err != nil {
		panic(fmt.Errorf("getting git short commit id"))
	}
	shortCommitID = string(shortCommitIDBytes)

	version = os.Getenv("VERSION")
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
	apisTidyCmd.Dir = workDir + "/apis"
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
		mg.F(Build.cmd, "addon-operator-manager"),
		mg.F(Build.cmd, "addon-operator-webhook"),
		mg.F(Build.cmd, "api-mock"),
	)
}

// Builds the docgen internal tool
func (Build) Docgen() {
	mg.Deps(mg.F(Build.cmd, "docgen"))
}

// Builds binaries from /cmd directory.
func (Build) cmd(cmd string) error {
	mg.Deps(
		Generators.code,
	)

	if err := sh.RunWithV(
		map[string]string{
			"GOARGS":      "GOOS=linux GOARCH=amd64",
			"GOFLAGS":     "",
			"CGO_ENABLED": "0",
			"LDFLAGS":     ldFlags,
		},
		"go", "build", "-v", "-o", "bin/"+cmd, "./cmd/"+cmd+"/main.go",
	); err != nil {
		return fmt.Errorf("compiling cmd/%s: %w", cmd, err)
	}
	return nil
}

// Testing and Linting
// -------------------

// Runs code-generators, checks for clean directory and lints the source code.
func Lint() error {
	mg.Deps(Generate)

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

func (d Dependency) kind() {
	mg.Deps(
		mg.F(Dependency.goInstall, "kind",
			"sigs.k8s.io/kind", kindVersion),
	)
}

func (d Dependency) controllerGen() {
	mg.Deps(
		mg.F(Dependency.goInstall, "controller-gen",
			"sigs.k8s.io/controller-tools/cmd/controller-gen", controllerGenVersion),
	)
}

func (d Dependency) yq() {
	mg.Deps(
		mg.F(Dependency.goInstall, "yq",
			"github.com/mikefarah/yq/v4", yqVersion),
	)
}

func (d Dependency) goimports() {
	mg.Deps(
		mg.F(Dependency.goInstall, "go-imports",
			"golang.org/x/tools/cmd/goimports", goimportsVersion),
	)
}

func (d Dependency) golangciLint() {
	mg.Deps(
		mg.F(Dependency.goInstall, "golangci-lint",
			"github.com/golangci/golangci-lint/cmd/golangci-lint", golangciLintVersion),
	)
}

func (d Dependency) opm() error {
	mg.Deps(Dependency.dirs)

	needsRebuild, err := d.needsRebuild("opm", opmVersion)
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
	if err := sh.Run(
		"curl", "-L", "--fail",
		"-o", tempDir+"/opm",
		fmt.Sprintf(
			"https://github.com/protocolbuffers/protobuf/releases/download/%s/linux-amd64-opm",
			opmVersion,
		),
	); err != nil {
		return fmt.Errorf("downloading protoc: %w", err)
	}

	// Move
	if err := os.Rename(tempDir+"/opm", depsDir+"/bin/opm"); err != nil {
		return fmt.Errorf("move protoc: %w", err)
	}
	return nil
}

// Installs a tool from the packageURL and given version.
// Will remember the version the tool was last installed as and not try to reinstall again until the version changed.
func (d Dependency) goInstall(tool, packageURl, version string) error {
	mg.Deps(Dependency.dirs)

	needsRebuild, err := d.needsRebuild(tool, version)
	if err != nil {
		return err
	}
	if !needsRebuild {
		return nil
	}

	url := packageURl + "@v" + version
	if err := sh.RunWithV(map[string]string{
		"GOBIN": depsDir + "/bin",
	}, mg.GoCmd(),
		"install", url,
	); err != nil {
		return fmt.Errorf("install %s: %w", url, err)
	}
	return nil
}

// Always required directories
func (Dependency) dirs() error {
	for _, dir := range []string{
		cacheDir, depsDir,
	} {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return fmt.Errorf("creating %q directory: %w", dir, err)
		}
	}

	return nil
}

func (Dependency) needsRebuild(tool, version string) (needsRebuild bool, err error) {
	versionFile := fmt.Sprintf(".deps/versions/%s/v%s", tool, version)
	if err := ensureFile(versionFile); err != nil {
		return false, fmt.Errorf("ensure file: %w", err)
	}

	// Checks "tool" binary file modification date against version file.
	// If the version file is newer, tool is of the wrong version.
	rebuild, err := target.Path(".deps/bin/"+tool, versionFile)
	if err != nil {
		return false, fmt.Errorf("rebuild check: %w", err)
	}

	return rebuild, nil
}

// ensure a file and it's file path exist.
func ensureFile(file string) error {
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		f, err := os.Create(file)
		if err != nil {
			return fmt.Errorf("creating file %s: %w", file, err)
		}
		defer f.Close()
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking file %s: %w", file, err)
	}
	return nil
}
