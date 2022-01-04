package dev

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sort"

	configv1 "github.com/openshift/api/config/v1"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	aoapis "github.com/openshift/addon-operator/apis"
)

// Container object to hold kubernetes client interfaces and configuration.
type KubernetesCluster struct {
	Scheme     *runtime.Scheme
	Config     *rest.Config
	CtrlClient client.Client
}

// Creates a new KubernetesCluster object to interact with a Kubernetes cluster.
func NewKubernetesCluster(kubeconfig string) (*KubernetesCluster, error) {
	scheme := runtime.NewScheme()

	sb := runtime.SchemeBuilder{
		clientgoscheme.AddToScheme,
		aoapis.AddToScheme,
		apiextensionsv1.AddToScheme,
		operatorsv1.AddToScheme,
		operatorsv1alpha1.AddToScheme,
		configv1.AddToScheme,
		monitoringv1.AddToScheme,
	}
	if err := sb.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding to scheme: %w", err)
	}

	kubeconfigBytes, err := ioutil.ReadFile(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("reading kubeconfig %s: %w", kubeconfig, err)
	}
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfigBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing kubeconfig %s: %w", kubeconfig, err)
	}

	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("getting rest.Config from ClientConfig: %w", err)
	}

	ctrlClient, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("creating new ctrl client: %w", err)
	}

	return &KubernetesCluster{
		Scheme:     scheme,
		Config:     config,
		CtrlClient: ctrlClient,
	}, nil
}

// Loads kubernets objects from all .yaml files in the given folder.
// Does not recurse into subfolders.
// Preserves lexical file order.
func LoadKubernetesObjectsFromFolder(folderPath string) ([]unstructured.Unstructured, error) {
	folder, err := os.Open(folderPath)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", folderPath, err)
	}

	files, err := folder.Readdir(-1)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}
	sort.Sort(FileInfosByName(files))

	var objects []unstructured.Unstructured
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if path.Ext(file.Name()) != ".yaml" {
			continue
		}

		objs, err := LoadKubernetesObjectsFromFile(path.Join(folderPath, file.Name()))
		if err != nil {
			return nil, fmt.Errorf("loading kubernetes objects from file %q: %w", file, err)
		}
		objects = append(objects, objs...)
	}
	return objects, nil
}

// Loads a kubernetes objects from the given file.
// A single fiel may contain multiple objects separated by "---\n".
func LoadKubernetesObjectsFromFile(filePath string) ([]unstructured.Unstructured, error) {
	fileYaml, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filePath, err)
	}

	// Trim empty starting and ending objects
	fileYaml = bytes.Trim(fileYaml, "---\n")

	var objects []unstructured.Unstructured
	// Split for every included yaml document.
	for i, yamlDocument := range bytes.Split(fileYaml, []byte("---\n")) {
		obj := unstructured.Unstructured{}
		if err := yaml.Unmarshal(yamlDocument, &obj); err != nil {
			return nil, fmt.Errorf(
				"unmarshalling yaml document from file %q at index %d: %w", filePath, i, err)
		}

		objects = append(objects, obj)
	}

	return objects, nil
}
