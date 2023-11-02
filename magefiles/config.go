//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"path"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"github.com/mt-sre/devkube/dev"
	"github.com/mt-sre/devkube/magedeps"
)

var (
	logger           logr.Logger = stdr.New(nil)
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

// setupContainerRuntime is a dependency for all targets requiring a container runtime
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
