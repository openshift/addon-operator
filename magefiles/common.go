//go:build mage
// +build mage

package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/mt-sre/devkube/magedeps"
)

const (
	module          = "github.com/openshift/addon-operator"
	defaultImageOrg = "quay.io/app-sre"
)

var (
	workDir  string                       // Working directory of the project
	depsDir  magedeps.DependencyDirectory // Dependency directory
	cacheDir string
)

func gnuSed() bool {
	cmd := exec.Command("sed", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("warning: sed --version returned error. this could mean that 'sed' is not 'gnu sed':", string(output), err)
		return false
	}
	return strings.Contains(string(output), "GNU")
}
