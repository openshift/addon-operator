package dev

import (
	"bytes"
	"context"
	goerrors "errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"sort"
	"time"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/yaml"

	aoapis "github.com/openshift/addon-operator/apis"
)

// Container object to hold kubernetes client interfaces and configuration.
type KubernetesCluster struct {
	Scheme     *runtime.Scheme
	Config     *rest.Config
	CtrlClient client.Client
	log        logr.Logger
}

// Creates a new KubernetesCluster object to interact with a Kubernetes cluster.
func NewKubernetesCluster(kubeconfig string, log logr.Logger) (*KubernetesCluster, error) {
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

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("getting rest.Config from kubeconfig: %w", err)
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
		log:        log,
	}, nil
}

type UnknownTypeError struct {
	GK schema.GroupKind
}

func (e *UnknownTypeError) Error() string {
	return fmt.Sprintf("unknown type: %s", e.GK)
}

// Load kube objects from a list of http urls,
// create these objects and wait for them to be ready.
func (c *KubernetesCluster) CreateAndWaitFromHttp(
	ctx context.Context, urls []string) error {
	var client http.Client
	var objects []unstructured.Unstructured
	for _, url := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("getting %q: %w", url, err)
		}
		defer resp.Body.Close()

		var content bytes.Buffer
		if _, err := io.Copy(&content, resp.Body); err != nil {
			return fmt.Errorf("reading response %q: %w", url, err)
		}

		objs, err := LoadKubernetesObjectsFromBytes(content.Bytes())
		if err != nil {
			return fmt.Errorf("loading objects from %q: %w", url, err)
		}

		objects = append(objects, objs...)
	}

	for i := range objects {
		if err := c.CreateAndWaitForReadiness(
			ctx, DefaultWaitTimeout, &objects[i]); err != nil {
			return fmt.Errorf("creating object: %w", err)
		}
	}
	return nil
}

// Load kube objects from a list of files,
// create these objects and wait for them to be ready.
func (c *KubernetesCluster) CreateAndWaitFromFiles(
	ctx context.Context, files []string) error {
	var objects []unstructured.Unstructured
	for _, file := range files {
		objs, err := LoadKubernetesObjectsFromFile(file)
		if err != nil {
			return fmt.Errorf("loading objects from file %q: %w", file, err)
		}

		objects = append(objects, objs...)
	}

	for i := range objects {
		if err := c.CreateAndWaitForReadiness(
			ctx, DefaultWaitTimeout, &objects[i]); err != nil {
			return fmt.Errorf("creating object: %w", err)
		}
	}
	return nil
}

// Load kube objects from a list of folders,
// create these objects and wait for them to be ready.
func (c *KubernetesCluster) CreateAndWaitFromFolders(
	ctx context.Context, folders []string) error {
	var objects []unstructured.Unstructured
	for _, folder := range folders {
		objs, err := LoadKubernetesObjectsFromFolder(folder)
		if err != nil {
			return fmt.Errorf("loading objects from folder %q: %w", folder, err)
		}

		objects = append(objects, objs...)
	}

	for i := range objects {
		if err := c.CreateAndWaitForReadiness(
			ctx, DefaultWaitTimeout, &objects[i]); err != nil {
			return fmt.Errorf("creating object: %w", err)
		}
	}
	return nil
}

// Creates the given objects and waits for them to be considered ready.
func (c *KubernetesCluster) CreateAndWaitForReadiness(
	ctx context.Context, timeout time.Duration, object client.Object,
) error {
	if err := c.CtrlClient.Create(ctx, object); err != nil &&
		!errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating object: %w", err)
	}

	if err := c.WaitForReadiness(ctx, DefaultWaitTimeout, object); err != nil {
		var unknownTypeErr *UnknownTypeError
		if goerrors.As(err, &unknownTypeErr) {
			// A lot of types don't require waiting for readiness,
			// so we should not error in cases when object types
			// are not registered for the generic wait method.
			return nil
		}

		return fmt.Errorf("waiting for object: %w", err)
	}
	return nil
}

// Waits for an object to be considered available.
func (c *KubernetesCluster) WaitForReadiness(
	ctx context.Context, timeout time.Duration, object client.Object,
) error {
	gvk, err := apiutil.GVKForObject(object, c.Scheme)
	if err != nil {
		return fmt.Errorf("could not determine GVK for object: %w", err)
	}

	gk := gvk.GroupKind()
	switch gk {
	case schema.GroupKind{
		Kind: "Deployment", Group: "apps"}:
		return c.WaitForCondition(ctx, timeout, object, "Available", metav1.ConditionTrue)

	case schema.GroupKind{
		Kind: "CustomResourceDefinition", Group: "apiextensions.k8s.io"}:
		return c.WaitForCondition(ctx, timeout, object, "Established", metav1.ConditionTrue)

	default:
		return &UnknownTypeError{GK: gk}
	}
}

// Waits for an object to report the given condition with given status.
// Takes observedGeneration into account when present on the object.
// observedGeneration may be reported on the condition or under .status.observedGeneration.
func (c *KubernetesCluster) WaitForCondition(
	ctx context.Context, timeout time.Duration, object client.Object,
	conditionType string, conditionStatus metav1.ConditionStatus,
) error {
	return c.WaitForObject(
		ctx, timeout, object,
		fmt.Sprintf("to report condition %q=%q", conditionType, conditionStatus),
		func(obj client.Object) (done bool, err error) {
			return checkObjectCondition(obj, conditionType, conditionStatus, c.Scheme)
		})
}

// Wait for an object to match a check function.
func (c *KubernetesCluster) WaitForObject(
	ctx context.Context, timeout time.Duration,
	object client.Object, waitReason string,
	checkFn func(obj client.Object) (done bool, err error),
) error {
	gvk, err := apiutil.GVKForObject(object, c.Scheme)
	if err != nil {
		return err
	}

	key := client.ObjectKeyFromObject(object)
	c.log.Info(fmt.Sprintf("waiting %s on %s %s %s...",
		timeout, gvk, key, waitReason))

	return wait.PollImmediateWithContext(ctx, time.Second, timeout, func(ctx context.Context) (done bool, err error) {
		err = c.CtrlClient.Get(ctx, client.ObjectKeyFromObject(object), object)
		if err != nil {
			//nolint:nilerr // retry on transient errors
			return false, nil
		}

		return checkFn(object)
	})
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

// Loads kubernetes objects from the given file.
func LoadKubernetesObjectsFromFile(filePath string) ([]unstructured.Unstructured, error) {
	fileYaml, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filePath, err)
	}

	return LoadKubernetesObjectsFromBytes(fileYaml)
}

// Loads kubernetes objects from given bytes.
// A single file may contain multiple objects separated by "---\n".
func LoadKubernetesObjectsFromBytes(fileYaml []byte) ([]unstructured.Unstructured, error) {
	// Trim empty starting and ending objects
	fileYaml = bytes.Trim(fileYaml, "---\n")

	var objects []unstructured.Unstructured
	// Split for every included yaml document.
	for i, yamlDocument := range bytes.Split(fileYaml, []byte("---\n")) {
		obj := unstructured.Unstructured{}
		if err := yaml.Unmarshal(yamlDocument, &obj); err != nil {
			return nil, fmt.Errorf(
				"unmarshalling yaml document at index %d: %w", i, err)
		}

		objects = append(objects, obj)
	}

	return objects, nil
}

func checkObjectCondition(
	obj client.Object, conditionType string,
	conditionStatus metav1.ConditionStatus,
	scheme *runtime.Scheme,
) (done bool, err error) {
	unstrObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		unstrObj = &unstructured.Unstructured{}
		if err := scheme.Convert(obj, unstrObj, nil); err != nil {
			return false, fmt.Errorf("can't convert to unstructured: %w", err)
		}
	}

	if observedGen, ok, err := unstructured.NestedInt64(
		unstrObj.Object, "status", "observedGeneration"); err != nil {
		return false, fmt.Errorf("could not access .status.observedGeneration: %w", err)
	} else if ok && observedGen != obj.GetGeneration() {
		// Object status outdated
		return false, nil
	}

	conditionsRaw, ok, err := unstructured.NestedFieldCopy(
		unstrObj.Object, "status", "conditions")
	if err != nil {
		return false, fmt.Errorf("could not access .status.conditions: %w", err)
	}
	if !ok {
		// no conditions reported
		return false, nil
	}

	// Press into metav1.Condition scheme to be able to work typed.
	conditionsYaml, err := yaml.Marshal(conditionsRaw)
	if err != nil {
		return false, fmt.Errorf("could not marshal conditions into yaml: %w", err)
	}
	var conditions []metav1.Condition
	if err := yaml.Unmarshal(conditionsYaml, &conditions); err != nil {
		return false, fmt.Errorf("could not unmarshal conditions: %w", err)
	}

	// Check conditions
	condition := meta.FindStatusCondition(conditions, conditionType)
	if condition == nil {
		// no such condition
		return false, nil
	}

	if condition.ObservedGeneration != 0 &&
		condition.ObservedGeneration != obj.GetGeneration() {
		// Condition outdated
		return false, nil
	}

	return condition.Status == conditionStatus, nil
}
