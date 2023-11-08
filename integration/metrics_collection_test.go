package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/sethvargo/go-retry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/integration"
	"github.com/openshift/addon-operator/internal/controllers"
	"github.com/openshift/addon-operator/internal/testutil"
)

// Tests that reconcile error metrics are collected
func (s *integrationTestSuite) TestReconcileErrorMetrics() {
	if !testutil.IsApiMockEnabled() {
		s.T().Skip("skipping OCM tests since api mock execution is disabled")
	}

	ctx := context.Background()
	addon := addon_OwnNamespace()

	// 1. Configure api mock to return an error
	// on creating OCM endpoints for addon status
	err := configureApiMock(ctx, true)
	s.Require().NoError(err)

	// 2. Patch ADO so it uses the ocm client/api
	var ado1 addonsv1alpha1.AddonOperator
	err = integration.Client.Get(ctx, client.ObjectKey{
		Name:      "addon-operator",
		Namespace: "addon-operator",
	}, &ado1)
	s.Require().NoError(err)
	ado2 := ado1.DeepCopy()
	ado2.Spec.OCM = &addonsv1alpha1.AddonOperatorOCM{
		Endpoint: "http://api-mock.api-mock.svc.cluster.local",
		Secret: addonsv1alpha1.ClusterSecretReference{
			Name:      "pull-secret",
			Namespace: "api-mock",
		},
	}
	err = integration.Client.Patch(ctx, ado2, client.MergeFrom(&ado1))
	s.Require().NoError(err)

	// 3. Create an addon
	err = integration.Client.Create(ctx, addon)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		// Ensure cleanup of addon
		s.addonCleanup(addon, ctx)
		// Ensure to reset api-mock setting
		err = configureApiMock(ctx, false)
		s.Require().NoError(err)
	})

	// 4. Wait for addon to be created
	err = integration.WaitForObject(
		ctx,
		s.T(), defaultAddonAvailabilityTimeout, addon, "to be Available",
		func(obj client.Object) (done bool, err error) {
			a := obj.(*addonsv1alpha1.Addon)
			return meta.IsStatusConditionTrue(
				a.Status.Conditions, addonsv1alpha1.Available), nil
		})
	s.Require().NoError(err)

	// 5. Get addon-operator-manager pod
	adoPods := &corev1.PodList{}
	err = integration.Client.List(
		ctx,
		adoPods,
		client.InNamespace("addon-operator"),
	)
	s.Require().NoError(err)
	var adoPod corev1.Pod
	for _, pod := range adoPods.Items {
		if strings.HasPrefix(pod.Name, "addon-operator-manager") {
			adoPod = pod
			break
		}
	}
	s.Require().NotNil(adoPod)

	backoff := retry.NewConstant(time.Minute)
	err = retry.Do(
		ctx,
		retry.WithMaxDuration(time.Minute*3, backoff),
		func(ctx context.Context) error {
			metricNotFound := errors.New("expected addon_operator metric was not found")
			podCommand := []string{"curl", "https://localhost:8443/metrics", "-k"}
			// nolint:contextcheck
			result, _, err := integration.ExecCommandInPod(
				"addon-operator",
				adoPod.Name,
				"metrics-relay-server",
				podCommand,
			)
			s.Require().NoError(err)
			// 5.1 Ensure ErrOCMClientRequest error was recorded due to
			// the mock OCM API error instrumented previously.
			if strings.Contains(result, controllers.ErrOCMClientRequest.Error()) {
				return err
			}
			return retry.RetryableError(metricNotFound)
		})
	s.Require().NoError(err)
}

func getAPIMockPod(ctx context.Context) (corev1.Pod, error) {
	var apiMockPod corev1.Pod
	pods := &corev1.PodList{}
	err := integration.Client.List(
		ctx,
		pods,
		client.InNamespace("api-mock"),
	)
	if err != nil {
		return apiMockPod, err
	}
	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Name, "api-mock") {
			apiMockPod = pod
			break
		}
	}
	return apiMockPod, err
}

func configureApiMock(ctx context.Context, failOnAddonStatusCreateEndpoint bool) error {
	apiMockPod, err := getAPIMockPod(ctx)
	if err != nil {
		return err
	}
	fail := strconv.FormatBool(failOnAddonStatusCreateEndpoint)
	payload := map[string]string{
		"failOnAddonStatusCreateEndpoint": fail,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	podCommand := []string{
		"curl",
		"-X",
		"PATCH",
		"-H",
		"Content-Type: application/json",
		"-d",
		string(jsonPayload),
		"http://api-mock/configure",
		"-k",
	}
	backoff := retry.NewConstant(time.Second * 10)
	err = retry.Do(
		ctx,
		retry.WithMaxDuration(time.Minute*1, backoff),
		func(ctx context.Context) error {
			// nolint:contextcheck
			_, _, err := integration.ExecCommandInPod(
				"api-mock",
				apiMockPod.Name,
				apiMockPod.Spec.Containers[0].Name,
				podCommand,
			)
			if err != nil {
				return retry.RetryableError(err)
			}
			return err
		})
	return err
}
