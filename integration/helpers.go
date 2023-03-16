// Package integration contains the Addon Operator integration tests.
package integration

import (
	"bytes"
	"context"
	goerrors "errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/proxy"
	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"

	aoapis "github.com/openshift/addon-operator/apis"
	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/ocm"
)

const (
	OCMAPIEndpoint = "http://api-mock.api-mock.svc.cluster.local"
)

var (
	// Client pointing to the e2e test cluster.
	Client    client.Client
	Config    *rest.Config
	Scheme    = runtime.NewScheme()
	Cv        *configv1.ClusterVersion
	OCMClient *ocm.Client
	// Namespace that the Addon Operator is running in.
	// Needs to be auto-discovered, because OpenShift CI is installing the Operator in a non deterministic namespace.
	AddonOperatorNamespace string

	// Typed K8s Clients
	CoreV1Client corev1client.CoreV1Interface
)

func init() {
	// Client/Scheme setup.
	AddToSchemes := runtime.SchemeBuilder{
		clientgoscheme.AddToScheme,
		aoapis.AddToScheme,
		apiextensionsv1.AddToScheme,
		operatorsv1.AddToScheme,
		operatorsv1alpha1.AddToScheme,
		configv1.AddToScheme,
		monitoringv1.AddToScheme,
		obov1alpha1.AddToScheme,
		pkov1alpha1.AddToScheme,
	}
	if err := AddToSchemes.AddToScheme(Scheme); err != nil {
		panic(fmt.Errorf("could not load schemes: %w", err))
	}

	Config = ctrl.GetConfigOrDie()

	var err error
	Client, err = client.New(Config, client.Options{
		Scheme: Scheme,
	})
	if err != nil {
		panic(fmt.Errorf("creating runtime client: %w", err))
	}

	// Typed Kubernetes Clients
	CoreV1Client = corev1client.NewForConfigOrDie(Config)
	ctx := context.Background()

	// Get the OCP cluster version object
	Cv = &configv1.ClusterVersion{}
	if err := Client.Get(ctx, client.ObjectKey{Name: "version"}, Cv); err != nil {
		panic(fmt.Errorf("getting clusterversion: %w", err))
	}

	// discover AddonOperator Namespace
	deploymentList := &appsv1.DeploymentList{}
	// We can't use a label-selector, because OLM is overriding the deployment labels...
	if err := Client.List(ctx, deploymentList); err != nil {
		panic(fmt.Errorf("listing addon-operator deployments on the cluster: %w", err))
	}
	var addonOperatorDeployments []appsv1.Deployment
	for _, deployment := range deploymentList.Items {
		if deployment.Name == "addon-operator-manager" {
			addonOperatorDeployments = append(addonOperatorDeployments, deployment)
		}
	}
	switch len(addonOperatorDeployments) {
	case 0:
		panic(fmt.Errorf("no AddonOperator deployment found on the cluster!"))
	case 1:
		AddonOperatorNamespace = addonOperatorDeployments[0].Namespace
	default:
		panic(fmt.Errorf("multiple AddonOperator deployments found on the cluster!"))
	}
}

func InitOCMClient() error {
	// Create a client to talk with the OCM mock API for testing
	ocmClient, err := ocm.NewClient(
		context.Background(),
		ocm.WithEndpoint("http://127.0.0.1:8001/api/v1/namespaces/api-mock/services/api-mock:80/proxy"),
		ocm.WithAccessToken("accessToken"), //TODO: Needs to be supplied from the outside, does not matter for mock.
		ocm.WithClusterExternalID(string(Cv.Spec.ClusterID)),
	)
	if err != nil {
		return fmt.Errorf("initializing ocm client: %w", err)
	}
	OCMClient = ocmClient
	return nil
}

// Prints the phase of a pod together with the logs of every container.
func PrintPodStatusAndLogs(namespace string) error {
	ctx := context.Background()

	pods := &corev1.PodList{}
	if err := Client.List(ctx, pods, client.InNamespace(namespace)); err != nil {
		return err
	}

	for _, pod := range pods.Items {
		p := pod
		if err := reportPodStatus(ctx, &p); err != nil {
			return err
		}
	}
	return nil
}

func reportPodStatus(ctx context.Context, pod *corev1.Pod) error {
	fmt.Println("-----------------------------------------------------------")
	fmt.Printf("Pod %s: %s\n", client.ObjectKeyFromObject(pod), pod.Status.Phase)
	fmt.Println("-----------------------------------------------------------")

	for _, container := range pod.Spec.Containers {
		fmt.Printf("Container logs for: %s\n", container.Name)

		req := CoreV1Client.Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container: container.Name,
		})
		logs, err := req.Stream(ctx)
		if err != nil {
			return err
		}
		defer logs.Close()
		if _, err := io.Copy(os.Stdout, logs); err != nil {
			return err
		}
		fmt.Println("-----------------------------------------------------------")
	}
	return nil
}

// Default Interval in which to recheck wait conditions.
const defaultWaitPollInterval = time.Second

// WaitToBeGone blocks until the given object is gone from the kubernetes API server.
func WaitToBeGone(ctx context.Context, t *testing.T, timeout time.Duration, object client.Object) error {
	gvk, err := apiutil.GVKForObject(object, Scheme)
	if err != nil {
		return err
	}

	key := client.ObjectKeyFromObject(object)
	t.Logf("waiting %s for %s %s to be gone...",
		timeout, gvk, key)

	return wait.PollImmediate(defaultWaitPollInterval, timeout, func() (done bool, err error) {
		err = Client.Get(ctx, key, object)

		if errors.IsNotFound(err) {
			return true, nil
		}

		if err != nil {
			t.Logf("error waiting for %s %s to be gone: %v",
				object.GetObjectKind().GroupVersionKind().Kind, key, err)
		}
		return false, nil
	})
}

// Wait that something happens with an object.
func WaitForObject(
	ctx context.Context,
	t *testing.T, timeout time.Duration,
	object client.Object, reason string,
	checkFn func(obj client.Object) (done bool, err error),
) error {
	gvk, err := apiutil.GVKForObject(object, Scheme)
	if err != nil {
		return err
	}

	key := client.ObjectKeyFromObject(object)
	t.Logf("waiting %s on %s %s %s...",
		timeout, gvk, key, reason)

	return wait.PollImmediate(time.Second, timeout, func() (done bool, err error) {
		err = Client.Get(ctx, client.ObjectKeyFromObject(object), object)
		if err != nil {
			//nolint:nilerr // retry on transient errors
			return false, nil
		}

		return checkFn(object)
	})
}

// Wait for an up-to-date condition value on an addon.
// A condition is considered up-to-date when it's .ObservedGeneration
// matches the generation of it's addon object.
func WaitForFreshAddonCondition(
	t *testing.T, timeout time.Duration,
	a *addonsv1alpha1.Addon, conditionType string, conditionStatus metav1.ConditionStatus,
) error {
	ctx := context.Background()
	return WaitForObject(
		ctx,
		t, timeout, a, fmt.Sprintf("to be %s: %s", conditionType, conditionStatus),
		func(obj client.Object) (done bool, err error) {
			a := obj.(*addonsv1alpha1.Addon)
			return isFreshStatusCondition(a, conditionType, conditionStatus), nil
		})
}

// isFreshStatusCondition returns true when `conditionType` is present,
// it's status matches `conditionStatus`
// and `.ObservedGeneration` matches `addon.ObjectMeta.Generation`
func isFreshStatusCondition(a *addonsv1alpha1.Addon, conditionType string, conditionStatus metav1.ConditionStatus) bool {
	for _, condition := range a.Status.Conditions {
		if condition.Type != conditionType {
			continue
		}

		if condition.Status != conditionStatus {
			return false
		}

		return condition.ObservedGeneration == a.GetGeneration()
	}

	return false
}

const (
	defaultPort      = 8001
	defaultAPIPrefix = "/"
	defaultAddress   = "127.0.0.1"
)

// Runs a local apiserver proxy on 127.0.0.1:8001 similar to `kubectl proxy`.
func RunAPIServerProxy(closeCh <-chan struct{}) error {
	mux := http.NewServeMux()

	proxyHandler, err := proxy.NewProxyHandler(defaultAPIPrefix, nil, Config, 0, false)
	if err != nil {
		return fmt.Errorf("creating proxy server: %w", err)
	}
	mux.Handle(defaultAPIPrefix, proxyHandler)

	// Already start a listener, so callers can already connect to the server,
	// even if the server is not up yet.
	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", defaultAddress, defaultPort))
	if err != nil {
		return fmt.Errorf("listen on %s:%d: %w", defaultAddress, defaultPort, err)
	}

	//nolint: gosec
	server := http.Server{Handler: mux}

	go func() {
		if err := server.Serve(l); err != nil &&
			!goerrors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()

	go func() {
		<-closeCh
		if err := server.Close(); err != nil {
			panic(err)
		}
	}()
	return nil
}

func ExecCommandInPod(namespace string, pod string, container string, command []string) (string, string, error) {
	ctx := context.Background()

	attachOptions := &corev1.PodExecOptions{
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
		Container: container,
		Command:   command,
	}

	request := CoreV1Client.RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(attachOptions, clientgoscheme.ParameterCodec)

	stdoutStream, stderrStream := new(bytes.Buffer), new(bytes.Buffer)
	streamOptions := remotecommand.StreamOptions{
		Stdout: stdoutStream,
		Stderr: stderrStream,
	}

	exec, err := remotecommand.NewSPDYExecutor(Config, "POST", request.URL())
	if err != nil {
		return "", "", fmt.Errorf("failed to establish SPDY stream with Kubernetes API: %w", err)
	}

	err = exec.StreamWithContext(ctx, streamOptions)
	stdout, stderr := strings.TrimSpace(stdoutStream.String()), strings.TrimSpace(stderrStream.String())
	if err != nil {
		return "", "", fmt.Errorf("failed to transport shell streams: %w\n%s", err, stderr)
	}

	return stdout, stderr, nil
}
