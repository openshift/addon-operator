//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"path"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Dependency Versions
const (
	kindVersion        = "0.20.0"
	yqVersion          = "4.35.1"
	goimportsVersion   = "0.12.0"
	olmVersion         = "0.20.0"
	opmVersion         = "1.24.0"
	pkoCliVersion      = "1.6.1"
	helmVersion        = "3.12.2"
	OperatorSDKVersion = "1.29.0"
)

type Dependency mg.Namespace

func (d Dependency) All() {
	mg.Deps(
		Dependency.Kind,
		Dependency.YQ,
		Dependency.Goimports,
		Dependency.Helm,
		Dependency.Opm,
		Dependency.OperatorSDK,
	)
}

// Ensure Kind dependency - Kubernetes in Docker (or Podman)
func (d Dependency) Kind() error {
	return depsDir.GoInstall("kind",
		"sigs.k8s.io/kind", kindVersion)
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

func (d Dependency) PkoCli() error {
	if err := os.MkdirAll(depsDir.Bin(), os.ModePerm); err != nil {
		return fmt.Errorf("create dependency dir: %w", err)
	}

	needsRebuild, err := depsDir.NeedsRebuild("kubectl-package", pkoCliVersion)
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
	tempPkoCliBin := path.Join(tempDir, "kubectl-package")
	if err := sh.RunV(
		"curl", "-L", "--fail",
		"-o", tempPkoCliBin,
		fmt.Sprintf(
			"https://github.com/package-operator/package-operator/releases/download/v%s/kubectl-package_linux_amd64",
			pkoCliVersion,
		),
	); err != nil {
		return fmt.Errorf("downloading kubectl-package: %w", err)
	}

	if err := os.Chmod(tempPkoCliBin, 0755); err != nil {
		return fmt.Errorf("make kubectl-package executable: %w", err)
	}

	// Move
	if err := os.Rename(tempPkoCliBin, path.Join(depsDir.Bin(), "kubectl-package")); err != nil {
		return fmt.Errorf("move kubectl-package: %w", err)
	}
	return nil
}

func (d Dependency) OperatorSDK() error {
	if err := os.MkdirAll(depsDir.Bin(), os.ModePerm); err != nil {
		return fmt.Errorf("create dependency dir: %w", err)
	}

	needsRebuild, err := depsDir.NeedsRebuild("operator-sdk", OperatorSDKVersion)
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
	operatorsdkBIN := path.Join(tempDir, "operator-sdk")
	if err := sh.RunV(
		"curl", "-L", "--fail",
		"-o", operatorsdkBIN,
		fmt.Sprintf(
			"https://github.com/operator-framework/operator-sdk/releases/download/v%s/operator-sdk_linux_amd64",
			OperatorSDKVersion,
		),
	); err != nil {
		return fmt.Errorf("downloading operator-sdk : %w", err)
	}

	if err := os.Chmod(operatorsdkBIN, 0755); err != nil {
		return fmt.Errorf("make operator-sdk executable: %w", err)
	}

	// Move
	if err := os.Rename(operatorsdkBIN, path.Join(depsDir.Bin(), "operator-sdk")); err != nil {
		return fmt.Errorf("move operator-sdk: %w", err)
	}
	return nil
}
