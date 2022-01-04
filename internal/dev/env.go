package dev

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Environment represents a development environment.
type Environment struct {
	Config  EnvironmentConfig
	Cluster *KubernetesCluster
}

// Creates a new development environment.
func NewEnvironment(name string, opts ...EnvironmentOption) *Environment {
	config := EnvironmentConfig{
		Name: name,
	}
	for _, opt := range opts {
		opt(&config)
	}
	return &Environment{
		Config: config,
	}
}

// Initializes the environment and prepares it for use.
func (env *Environment) Init(ctx context.Context) error {
	// Create cluster
	kindCmd := exec.CommandContext( //nolint:gosec
		ctx, "kind", "create", "cluster",
		"--kubeconfig="+env.Config.KubeconfigPath, "--name="+env.Config.Name,
	)
	kindCmd.Stdout = os.Stdout
	err := kindCmd.Run()
	if err != nil && strings.Contains(
		err.Error(), "already exist for a cluster with the name") {
		return fmt.Errorf("creating kind cluster: %w", err)
	}

	// Create _all_ the clients
	cluster, err := NewKubernetesCluster(env.Config.KubeconfigPath)
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

type EnvironmentOption func(c *EnvironmentConfig)

func EnvironmentWithClusterInitializers(init ...ClusterInitializer) EnvironmentOption {
	return func(c *EnvironmentConfig) {
		c.ClusterInitializers = append(c.ClusterInitializers, init...)
	}
}

type EnvironmentConfig struct {
	// Name of the environment.
	Name string
	// Cluster initializers prepare a cluster for use.
	ClusterInitializers []ClusterInitializer
	// Path to the kubeconfig for this cluster.
	KubeconfigPath string
}

// Apply default configuration.
func (c *EnvironmentConfig) Default() {
	if len(c.KubeconfigPath) == 0 {
		c.KubeconfigPath = ".cache/" + c.Name + "/kubeconfig.yaml"
	}
}

type ClusterInitializer interface {
	Init(ctx context.Context, cluster *KubernetesCluster) error
}

// Load objects from given folder paths and applies them into the cluster.
type ClusterLoadObjectsFromFolder []string

func (l ClusterLoadObjectsFromFolder) Init(
	ctx context.Context, cluster *KubernetesCluster) error {
	var objects []unstructured.Unstructured
	for _, folder := range l {
		objs, err := LoadKubernetesObjectsFromFolder(folder)
		if err != nil {
			return fmt.Errorf("loading objects from folder %q: %w", folder, err)
		}

		objects = append(objects, objs...)
	}

	for i := range objects {
		if err := cluster.CtrlClient.Create(ctx, &objects[i]); err != nil {
			return fmt.Errorf("creating object: %w", err)
		}
	}
	return nil
}

// Load objects from given file paths and applies them into the cluster.
type ClusterLoadObjectsFromFiles []string

func (l ClusterLoadObjectsFromFiles) Init(
	ctx context.Context, cluster *KubernetesCluster) error {
	var objects []unstructured.Unstructured
	for _, file := range l {
		objs, err := LoadKubernetesObjectsFromFile(file)
		if err != nil {
			return fmt.Errorf("loading objects from file %q: %w", file, err)
		}

		objects = append(objects, objs...)
	}

	for i := range objects {
		if err := cluster.CtrlClient.Create(ctx, &objects[i]); err != nil {
			return fmt.Errorf("creating object: %w", err)
		}
	}
	return nil
}
