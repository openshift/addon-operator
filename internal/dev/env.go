package dev

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/go-logr/stdr"
)

// Environment represents a development environment.
type Environment struct {
	Config  EnvironmentConfig
	Cluster *KubernetesCluster
}

// Creates a new development environment.
func NewEnvironment(name, workDir string, opts ...EnvironmentOption) *Environment {
	config := EnvironmentConfig{
		Name:    name,
		WorkDir: workDir,
	}
	for _, opt := range opts {
		opt(&config)
	}
	config.Default()
	return &Environment{
		Config: config,
	}
}

// Initializes the environment and prepares it for use.
func (env *Environment) Init(ctx context.Context) error {
	kindConfig := `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
`

	// Workaround for https://github.com/kubernetes-sigs/kind/issues/2411
	// For BTRFS on LUKS.
	if _, err := os.Lstat("/dev/dm-0"); err == nil {
		kindConfig += `nodes:
- role: control-plane
  extraMounts:
    - hostPath: /dev/dm-0
      containerPath: /dev/dm-0
      propagation: HostToContainer
`
	}

	if err := os.MkdirAll(env.Config.WorkDir, os.ModePerm); err != nil {
		return fmt.Errorf("creating workdir: %w", err)
	}

	kindConfigPath := env.Config.WorkDir + "/kind.yaml"
	if err := ioutil.WriteFile(
		kindConfigPath, []byte(kindConfig), os.ModePerm); err != nil {
		return fmt.Errorf("creating kind cluster config: %w", err)
	}

	// Needs cluster creation?
	var checkOutput bytes.Buffer
	if err := env.execKindCommand(ctx, &checkOutput, nil, "get", "clusters"); err != nil {
		return fmt.Errorf("getting existing kind clusters: %w", err)
	}
	// Only create cluster if it is not already there.
	if !strings.Contains(checkOutput.String(), env.Config.Name+"\n") {
		// Create cluster
		if err := env.execKindCommand(
			ctx, os.Stdout, os.Stderr,
			"create", "cluster",
			"--kubeconfig="+env.Config.KubeconfigPath, "--name="+env.Config.Name,
			"--config="+kindConfigPath,
		); err != nil {
			return fmt.Errorf("creating kind cluster: %w", err)
		}
	}

	// Create _all_ the clients
	cluster, err := NewKubernetesCluster(env.Config.KubeconfigPath, stdr.New(log.Default()))
	if err != nil {
		return fmt.Errorf("creating k8s clients: %w", err)
	}
	env.Cluster = cluster

	// Run ClusterInitializers
	for _, initializer := range env.Config.ClusterInitializers {
		if err := initializer.Init(ctx, cluster); err != nil {
			return fmt.Errorf("running cluster initializer: %w", err)
		}
	}

	return nil
}

// Destroy/Teardown the development environment.
func (env *Environment) Destroy(ctx context.Context) error {
	if err := env.execKindCommand(
		ctx, os.Stdout, os.Stderr,
		"delete", "cluster",
		"--kubeconfig="+env.Config.KubeconfigPath, "--name="+env.Config.Name,
	); err != nil {
		return fmt.Errorf("deleting kind cluster: %w", err)
	}
	return nil
}

// Load an image from a tar archive into the environment.
func (env *Environment) LoadImageFromTar(
	ctx context.Context, filePath string) error {
	if err := env.execKindCommand(
		ctx, os.Stdout, os.Stderr,
		"load", "image-archive", filePath,
		"--name="+env.Config.Name,
	); err != nil {
		return fmt.Errorf("loading image archive: %w", err)
	}
	return nil
}

func (env *Environment) execKindCommand(
	ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	kindCmd := exec.CommandContext( //nolint:gosec
		ctx, "kind", args...,
	)
	kindCmd.Env = os.Environ()
	if env.Config.ContainerRuntime == "podman" {
		kindCmd.Env = append(kindCmd.Env, "KIND_EXPERIMENTAL_PROVIDER=podman")
	}
	kindCmd.Stdout = stdout
	kindCmd.Stderr = stderr
	return kindCmd.Run()
}

type EnvironmentOption func(c *EnvironmentConfig)

func EnvironmentWithClusterInitializers(init ...ClusterInitializer) EnvironmentOption {
	return func(c *EnvironmentConfig) {
		c.ClusterInitializers = append(c.ClusterInitializers, init...)
	}
}

func EnvironmentWithContainerRuntime(containerRuntime string) EnvironmentOption {
	return func(c *EnvironmentConfig) {
		c.ContainerRuntime = containerRuntime
	}
}

type EnvironmentConfig struct {
	// Name of the environment.
	Name string
	// Working directory of the environment.
	// Temporary files/kubeconfig etc. will be stored here.
	WorkDir string
	// Path to the Kubeconfig
	KubeconfigPath string
	// Cluster initializers prepare a cluster for use.
	ClusterInitializers []ClusterInitializer
	// Container runtime to use
	ContainerRuntime string
}

// Apply default configuration.
func (c *EnvironmentConfig) Default() {
	if len(c.ContainerRuntime) == 0 {
		c.ContainerRuntime = "podman"
	}
	c.KubeconfigPath = c.WorkDir + "/kubeconfig.yaml"
}

type ClusterInitializer interface {
	Init(ctx context.Context, cluster *KubernetesCluster) error
}

// Load objects from given folder paths and applies them into the cluster.
type ClusterLoadObjectsFromFolder []string

func (l ClusterLoadObjectsFromFolder) Init(
	ctx context.Context, cluster *KubernetesCluster) error {
	return cluster.CreateAndWaitFromFolders(ctx, l)
}

// Load objects from given file paths and applies them into the cluster.
type ClusterLoadObjectsFromFiles []string

func (l ClusterLoadObjectsFromFiles) Init(
	ctx context.Context, cluster *KubernetesCluster) error {
	return cluster.CreateAndWaitFromFiles(ctx, l)
}

type ClusterLoadObjectsFromHttp []string

func (l ClusterLoadObjectsFromHttp) Init(
	ctx context.Context, cluster *KubernetesCluster) error {
	return cluster.CreateAndWaitFromHttp(ctx, l)
}
