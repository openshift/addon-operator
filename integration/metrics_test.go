package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/addon-operator/integration"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestMetricsEndpoint(t *testing.T) {
	t.Parallel()
	metrics := scrapeMetrics(t)
	require.NotNil(t, metrics)
}

func TestAddonMetrics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	addonMetrics := []string{
		"addon_operator_addons_phase_total",
		"addon_operator_addons_installation_total",
		"addon_operator_addons_installation_success_time_seconds",
	}
	// using random uuid so we can re-run test multiple times w/o metrics collision
	uuid := uuid.New()
	namespace := fmt.Sprintf("namespace-%s", uuid)
	trueAddonName := fmt.Sprintf("addon-%s", uuid)
	fakeAddonName := fmt.Sprintf("addon-fake-%s", uuid)
	addon := testutil.NewAddonOLMOwnNamespace(
		trueAddonName,
		namespace,
		referenceAddonCatalogSourceImageWorking,
	)

	require.False(t, metricsExistForAddon(t, addonMetrics, trueAddonName))
	require.False(t, metricsExistForAddon(t, addonMetrics, fakeAddonName))

	err := integration.Client.Create(ctx, addon)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := integration.Client.Delete(ctx, addon)
		if client.IgnoreNotFound(err) != nil {
			t.Logf("could not clean up Addon %s: %v", addon.Name, err)
		}
	})
	err = integration.WaitForAddonToBeAvailable(t, defaultAddonAvailabilityTimeout, addon)
	require.NoError(t, err)

	require.True(t, metricsExistForAddon(t, addonMetrics, trueAddonName))
	require.False(t, metricsExistForAddon(t, addonMetrics, fakeAddonName))
}

func metricsExistForAddon(t *testing.T, metricNames []string, addonName string) bool {
	metrics := scrapeMetrics(t)
	labelName := model.LabelName("name")
	labelValue := model.LabelValue(addonName)

	// make sure each metric family has metrics for the addon
	for _, k := range metricNames {
		found := false
		for _, sample := range extractSamples(t, metrics[k]) {
			labelSet := model.LabelSet(sample.Metric)
			if val, ok := labelSet[labelName]; ok && val == labelValue {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func scrapeMetrics(t *testing.T) map[string]*dto.MetricFamily {
	// endpoint will be available on localhost because of port-forwarding
	// see `$ make test-setup`
	endpoint := "http://127.0.0.1:8080/metrics"
	resp, err := http.Get(endpoint)
	require.NoError(t, err)

	defer resp.Body.Close()

	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(resp.Body)
	require.NoError(t, err)

	return mf
}

var decodeOpts = expfmt.DecodeOptions{Timestamp: model.Now()}

func extractSamples(t *testing.T, mf *dto.MetricFamily) model.Vector {
	res, err := expfmt.ExtractSamples(&decodeOpts, mf)
	require.NoError(t, err)
	return res
}
