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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

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
		panic(err)
	}

	// Typed Kubernetes Clients
	CoreV1Client = corev1client.NewForConfigOrDie(Config)

	Cv = &configv1.ClusterVersion{}
	if err := Client.Get(context.Background(), client.ObjectKey{Name: "version"}, Cv); err != nil {
		panic(fmt.Errorf("getting clusterversion: %w", err))
	}
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
func WaitToBeGone(t *testing.T, timeout time.Duration, object client.Object) error {
	gvk, err := apiutil.GVKForObject(object, Scheme)
	if err != nil {
		return err
	}

	key := client.ObjectKeyFromObject(object)
	t.Logf("waiting %s for %s %s to be gone...",
		timeout, gvk, key)

	ctx := context.Background()
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

	ctx := context.Background()
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
	return WaitForObject(
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

	server := http.Server{
		Handler: mux,
	}

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

	err = exec.Stream(streamOptions)
	stdout, stderr := strings.TrimSpace(stdoutStream.String()), strings.TrimSpace(stderrStream.String())
	if err != nil {
		return stdout, stderr, fmt.Errorf("failed to transport shell streams: %w", err)
	}

	return stdout, stderr, nil
}
