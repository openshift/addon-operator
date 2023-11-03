//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/magefile/mage/sh"
	olmversion "github.com/operator-framework/api/pkg/lib/version"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"sigs.k8s.io/yaml"
)

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
