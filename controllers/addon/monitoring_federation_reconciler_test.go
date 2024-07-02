package addon

import (
	"context"
	"fmt"
	"testing"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestEnsureMonitoringFederation_MonitoringFullyMissingInSpec_NotPresentInCluster(t *testing.T) {
	c := testutil.NewClient()

	uncachedC := testutil.NewClient()

	addon := testutil.NewTestAddonWithoutNamespace()

	r := &monitoringFederationReconciler{
		client:                 c,
		uncachedClient:         uncachedC,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := context.Background()
	_, err := r.ensureMonitoringFederation(ctx, addon)
	require.NoError(t, err)
	c.AssertExpectations(t)
}

func TestEnsureMonitoringFederation_MonitoringPresentInSpec_NotPresentInCluster(t *testing.T) {
	c := testutil.NewClient()
	uncachedC := testutil.NewClient()
	r := &monitoringFederationReconciler{
		client:                 c,
		uncachedClient:         uncachedC,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}

	addon := testutil.NewTestAddonWithMonitoringFederation()
	addon.Spec.Monitoring.Federation.PortName = "https"

	c.On("Get", testutil.IsContext, mock.IsType(types.NamespacedName{}), mock.IsType(&corev1.Namespace{}), mock.Anything).
		Return(testutil.NewTestErrNotFound())

	c.On("Create", testutil.IsContext, mock.IsType(&corev1.Namespace{}), mock.Anything).
		Run(func(args mock.Arguments) {
			// mocked Namespace is immediately active
			namespace := args.Get(1).(*corev1.Namespace)
			namespace.Status.Phase = corev1.NamespaceActive
			assert.Equal(t, GetMonitoringNamespaceName(addon), namespace.Name)
		}).
		Return(nil)

	secretADOtoken := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "addon-operator-prom-token",
			Namespace: r.addonOperatorNamespace,
		},
		Data: map[string][]byte{
			"token": []byte("mock-token"),
		},
	}

	uncachedC.
		On("Get",
			testutil.IsContext,
			mock.Anything,
			mock.IsType(&corev1.Secret{}), mock.Anything).Run(func(args mock.Arguments) {
		secretADOtoken.DeepCopyInto(args.Get(2).(*corev1.Secret))
	}).
		Return(nil)

	uncachedC.
		On("Get",
			testutil.IsContext,
			mock.Anything,
			mock.IsType(&corev1.Secret{}), mock.Anything).
		Return(testutil.NewTestErrNotFound())

	c.
		On("Create", mock.Anything, mock.IsType(&corev1.Secret{}), mock.Anything).Return(nil).Maybe()

	c.On("Update", mock.Anything, mock.IsType(&corev1.Secret{}), mock.Anything).Return(nil)

	c.On("Create", testutil.IsContext, mock.IsType(&monitoringv1.ServiceMonitor{}), mock.Anything).
		Run(func(args mock.Arguments) {

			serviceMonitor := args.Get(1).(*monitoringv1.ServiceMonitor)
			assert.Equal(t, "https", serviceMonitor.Spec.Endpoints[0].Port)
			assert.Equal(t, GetMonitoringFederationServiceMonitorName(addon), serviceMonitor.Name)
			assert.Equal(t, GetMonitoringNamespaceName(addon), serviceMonitor.Namespace)

		}).
		Return(nil).Maybe()
	ctx := context.Background()
	_, err := r.ensureMonitoringFederation(ctx, addon)
	require.NoError(t, err)

	c.AssertExpectations(t)

}

func TestEnsureMonitoringFederation_MonitoringPresentInSpec_PresentInCluster(t *testing.T) {
	c := testutil.NewClient()
	uncachedC := testutil.NewClient()

	r := &monitoringFederationReconciler{
		client:                 c,
		uncachedClient:         uncachedC,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}

	addon := testutil.NewTestAddonWithMonitoringFederation()
	addon.Spec.Monitoring.Federation.PortName = "portName"

	c.On("Get", testutil.IsContext, mock.IsType(types.NamespacedName{}), mock.IsType(&corev1.Namespace{}), mock.Anything).
		Run(func(args mock.Arguments) {
			namespacedName := args.Get(1).(types.NamespacedName)
			assert.Equal(t, GetMonitoringNamespaceName(addon), namespacedName.Name)
			// mocked Namespace is immediately active
			namespace := args.Get(2).(*corev1.Namespace)
			namespace.Status.Phase = corev1.NamespaceActive
			// mocked Namespace is owned by Addon
			err := controllerutil.SetControllerReference(addon, namespace, r.scheme)
			// mocked Namespace has desired labels
			namespace.Labels = map[string]string{"openshift.io/cluster-monitoring": "true"}
			controllers.AddCommonLabels(namespace, addon)
			assert.NoError(t, err)
		}).
		Return(nil)

	c.On("Get", testutil.IsContext, mock.IsType(types.NamespacedName{}), mock.IsType(&monitoringv1.ServiceMonitor{}), mock.Anything).
		Run(func(args mock.Arguments) {
			namespacedName := args.Get(1).(types.NamespacedName)
			assert.Equal(t, GetMonitoringFederationServiceMonitorName(addon), namespacedName.Name)
			assert.Equal(t, GetMonitoringNamespaceName(addon), namespacedName.Namespace)
			// mocked ServiceMonitor is owned by Addon
			serviceMonitor := args.Get(2).(*monitoringv1.ServiceMonitor)
			controllers.AddCommonLabels(serviceMonitor, addon)
			err := controllerutil.SetControllerReference(addon, serviceMonitor, r.scheme)
			assert.NoError(t, err)
			// inject expected ServiceMonitor spec into response
			serviceMonitor.Spec = monitoringv1.ServiceMonitorSpec{
				Endpoints: []monitoringv1.Endpoint{
					{
						Authorization: &monitoringv1.SafeAuthorization{Type: "Bearer", Credentials: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "addon-foo-bearertoken-secret"}, Key: "token"}},
						HonorLabels:   true,
						Port:          "portName",
						Path:          "/federate",
						Scheme:        "https",
						Params: map[string][]string{
							"match[]": {
								`ALERTS{alertstate="firing"}`,
								`{__name__="foo"}`,
							},
						},
						Interval: "30s",
						TLSConfig: &monitoringv1.TLSConfig{
							CAFile: "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
							SafeTLSConfig: monitoringv1.SafeTLSConfig{
								ServerName: fmt.Sprintf(
									"prometheus.%s.svc",
									addon.Spec.Monitoring.Federation.Namespace,
								),
							},
						},
					},
				},
				NamespaceSelector: monitoringv1.NamespaceSelector{
					MatchNames: []string{addon.Spec.Monitoring.Federation.Namespace},
				},
				Selector: metav1.LabelSelector{
					MatchLabels: addon.Spec.Monitoring.Federation.MatchLabels,
				},
			}
		}).
		Return(nil)

	uncachedC.
		On("Get",
			testutil.IsContext,
			mock.Anything,
			mock.IsType(&corev1.Secret{}), mock.Anything).
		Return(nil)

	uncachedC.
		On("Get",
			testutil.IsContext,
			mock.Anything,
			mock.IsType(&corev1.Secret{}), mock.Anything).
		Return(nil)

	c.
		On("Create", mock.Anything, mock.IsType(&corev1.Secret{}), mock.Anything).Return(nil)

	c.
		On("Update", mock.Anything, mock.IsType(&corev1.Secret{}), mock.Anything).Return(nil)

	c.On("Create", testutil.IsContext, mock.IsType(&monitoringv1.ServiceMonitor{}), mock.Anything).
		Run(func(args mock.Arguments) {
			serviceMonitor := args.Get(1).(*monitoringv1.ServiceMonitor)
			assert.Equal(t, "https", serviceMonitor.Spec.Endpoints[0].Port)
			assert.Equal(t, "Bearer", serviceMonitor.Spec.Endpoints[0].Authorization.Type)
			assert.Equal(t, "addon-foo-bearertoken-secret", serviceMonitor.Spec.Endpoints[0].Authorization.Credentials.LocalObjectReference.Name)
			assert.Equal(t, GetMonitoringFederationServiceMonitorName(addon), serviceMonitor.Name)
			assert.Equal(t, GetMonitoringNamespaceName(addon), serviceMonitor.Namespace)
		}).
		Return(nil)

	ctx := context.Background()
	_, err := r.ensureMonitoringFederation(ctx, addon)
	require.NoError(t, err)
}

func TestEnsureMonitoringFederation_Adoption(t *testing.T) {
	addon := testutil.NewTestAddonWithMonitoringFederation()

	for name, tc := range map[string]struct {
		ActualMonitoringNamespace *corev1.Namespace
		ActualServiceMonitor      *monitoringv1.ServiceMonitor
	}{
		"existing namespace with no owner": {
			ActualMonitoringNamespace: testMonitoringNamespace(addon),
			ActualServiceMonitor:      addonOwnedTestServiceMonitor(addon),
		},
		"existing serviceMonitor with no owner": {
			ActualMonitoringNamespace: addonOwnedTestMonitoringNamespace(addon),
			ActualServiceMonitor:      testServiceMonitor(addon),
		},
		"existing namespace and serviceMonitor addon owned": {
			ActualMonitoringNamespace: addonOwnedTestMonitoringNamespace(addon),
			ActualServiceMonitor:      addonOwnedTestServiceMonitor(addon),
		},
		"existing serviceMonitor with altered spec": {
			ActualMonitoringNamespace: addonOwnedTestMonitoringNamespace(addon),
			ActualServiceMonitor:      testServiceMonitorAlteredSpec(addon),
		},
		"existing serviceMonitor with altered spec and addon owned": {
			ActualMonitoringNamespace: addonOwnedTestMonitoringNamespace(addon),
			ActualServiceMonitor:      addonOwnedTestServiceMonitorAlteredSpec(addon),
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := testutil.NewClient()
			uncachedC := testutil.NewClient()
			rec := &monitoringFederationReconciler{
				client:         c,
				uncachedClient: uncachedC,

				scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
				addonOperatorNamespace: "xxx-addon-operator",
			}

			c.On("Get",
				testutil.IsContext,
				mock.IsType(types.NamespacedName{}),
				testutil.IsCoreV1NamespacePtr,
				mock.Anything).
				Run(func(args mock.Arguments) {
					tc.ActualMonitoringNamespace.DeepCopyInto(args.Get(2).(*corev1.Namespace))
				}).
				Return(nil)

			c.On("Update",
				testutil.IsContext,
				testutil.IsCoreV1NamespacePtr,
				mock.Anything).
				Return(nil).
				Maybe()

			c.On("Get",
				testutil.IsContext,
				mock.IsType(types.NamespacedName{}),
				testutil.IsMonitoringV1ServiceMonitorPtr,
				mock.Anything).
				Run(func(args mock.Arguments) {
					tc.ActualServiceMonitor.DeepCopyInto(args.Get(2).(*monitoringv1.ServiceMonitor))
				}).
				Return(nil).
				Maybe()

			c.On("Update",
				testutil.IsContext,
				testutil.IsMonitoringV1ServiceMonitorPtr,
				mock.Anything).
				Return(nil).
				Maybe()

			uncachedC.
				On("Get",
					testutil.IsContext,
					mock.Anything,
					mock.IsType(&corev1.Secret{}), mock.Anything).
				Return(nil)
			uncachedC.
				On("Get",
					testutil.IsContext,
					mock.Anything,
					mock.IsType(&corev1.Secret{}), mock.Anything).
				Return(nil)

			c.
				On("Create", mock.Anything, mock.IsType(&corev1.Secret{}), mock.Anything).Return(nil).Maybe()

			c.
				On("Update", mock.Anything, mock.IsType(&corev1.Secret{}), mock.Anything).Return(nil).Maybe()

			addonCopy := addon.DeepCopy()

			_, err := rec.ensureMonitoringFederation(context.Background(), addonCopy)
			assert.NoError(t, err)

			c.AssertExpectations(t)
		})
	}
}

func addonOwnedTestMonitoringNamespace(addon *addonsv1alpha1.Addon) *corev1.Namespace {
	ns := testMonitoringNamespace(addon)
	_ = controllerutil.SetControllerReference(addon, ns, testutil.NewTestSchemeWithAddonsv1alpha1())

	return ns
}

func testMonitoringNamespace(addon *addonsv1alpha1.Addon) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: GetMonitoringNamespaceName(addon),
		},
		Status: corev1.NamespaceStatus{
			Phase: corev1.NamespaceActive,
		},
	}
}

func addonOwnedTestServiceMonitor(addon *addonsv1alpha1.Addon) *monitoringv1.ServiceMonitor {
	sm := testServiceMonitor(addon)
	_ = controllerutil.SetControllerReference(addon, sm, testutil.NewTestSchemeWithAddonsv1alpha1())

	return sm
}

func addonOwnedTestServiceMonitorAlteredSpec(addon *addonsv1alpha1.Addon) *monitoringv1.ServiceMonitor {
	sm := testServiceMonitorAlteredSpec(addon)
	_ = controllerutil.SetControllerReference(addon, sm, testutil.NewTestSchemeWithAddonsv1alpha1())

	return sm
}

func testServiceMonitorAlteredSpec(addon *addonsv1alpha1.Addon) *monitoringv1.ServiceMonitor {
	serviceMonitor := testServiceMonitor(addon)
	sampleLimit := uint64(10)
	serviceMonitor.Spec.SampleLimit = &sampleLimit

	return serviceMonitor
}

func testServiceMonitor(addon *addonsv1alpha1.Addon) *monitoringv1.ServiceMonitor {
	existingBearerTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-bearertoken-secret", addon.Name),
			Namespace: addon.Namespace,
		},
		Data: map[string][]byte{
			"token": []byte("mock-token"),
		},
	}

	return &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetMonitoringFederationServiceMonitorName(addon),
			Namespace: GetMonitoringNamespaceName(addon),
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: GetMonitoringFederationServiceMonitorEndpoints(addon, existingBearerTokenSecret),
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{addon.Spec.Monitoring.Federation.Namespace},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: addon.Spec.Monitoring.Federation.MatchLabels,
			},
		},
	}
}
func TestEnsureDeletionOfMonitoringFederation_MonitoringFullyMissingInSpec_NotPresentInCluster(t *testing.T) {
	c := testutil.NewClient()

	addon := testutil.NewTestAddonWithoutNamespace()

	c.On("List", testutil.IsContext, mock.IsType(&monitoringv1.ServiceMonitorList{}), mock.Anything).
		Return(nil)
	c.On("Delete", testutil.IsContext, mock.IsType(&corev1.Namespace{}), mock.Anything).
		Run(func(args mock.Arguments) {
			ns := args.Get(1).(*corev1.Namespace)
			assert.Equal(t, GetMonitoringNamespaceName(addon), ns.Name)
		}).
		Return(testutil.NewTestErrNotFound())

	r := &monitoringFederationReconciler{
		client:                 c,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := context.Background()
	err := r.ensureDeletionOfUnwantedMonitoringFederation(ctx, addon)

	require.NoError(t, err)
	c.AssertExpectations(t)
}

func TestEnsureDeletionOfMonitoringFederation_MonitoringFullyMissingInSpec_PresentInCluster(t *testing.T) {
	c := testutil.NewClient()

	addon := testutil.NewTestAddonWithoutNamespace()

	serviceMonitorsInCluster := &monitoringv1.ServiceMonitorList{
		Items: []*monitoringv1.ServiceMonitor{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "qux",
					Namespace: "bar",
				},
			},
		},
	}
	deletedServiceMons := []client.ObjectKey{}

	c.On("List", testutil.IsContext, mock.IsType(&monitoringv1.ServiceMonitorList{}), mock.Anything).
		Run(func(args mock.Arguments) {
			list := args.Get(1).(*monitoringv1.ServiceMonitorList)
			serviceMonitorsInCluster.DeepCopyInto(list)
		}).
		Return(nil)
	c.On("Delete", testutil.IsContext, mock.IsType(&monitoringv1.ServiceMonitor{}), mock.Anything).
		Run(func(args mock.Arguments) {
			sm := args.Get(1).(*monitoringv1.ServiceMonitor)
			assert.Condition(t, func() (success bool) {
				for _, serviceMonitorInCluster := range serviceMonitorsInCluster.Items {
					if serviceMonitorInCluster.Name == sm.Name {
						return true
					}
				}
				return false
			})
			deletedServiceMons = append(deletedServiceMons, client.ObjectKeyFromObject(sm))
		}).
		Return(nil)
	c.On("Delete", testutil.IsContext, mock.IsType(&corev1.Namespace{}), mock.Anything).
		Run(func(args mock.Arguments) {
			ns := args.Get(1).(*corev1.Namespace)
			assert.Equal(t, GetMonitoringNamespaceName(addon), ns.Name)
		}).
		Return(nil)

	c.On("Delete", testutil.IsContext, mock.IsType(&corev1.Secret{}), mock.Anything).
		Run(func(args mock.Arguments) {
			ns := args.Get(1).(*corev1.Secret)
			assert.Equal(t, fmt.Sprintf("%s-bearertoken-secret", addon.Name), ns.Name)
		}).
		Return(nil)

	r := &monitoringFederationReconciler{
		client:                 c,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := context.Background()
	err := r.ensureDeletionOfUnwantedMonitoringFederation(ctx, addon)

	require.NoError(t, err)
	c.AssertExpectations(t)
	c.AssertCalled(t, "Delete", testutil.IsContext, mock.IsType(&corev1.Namespace{}), mock.Anything)
	assert.Equal(t, []client.ObjectKey{
		{Name: "foo", Namespace: "bar"},
		{Name: "qux", Namespace: "bar"},
	}, deletedServiceMons)
}

func TestEnsureDeletionOfMonitoringFederation_MonitoringFullyPresentInSpec_PresentInCluster(t *testing.T) {
	c := testutil.NewClient()

	addon := testutil.NewTestAddonWithMonitoringFederation()

	serviceMonitorsInCluster := &monitoringv1.ServiceMonitorList{
		Items: []*monitoringv1.ServiceMonitor{
			testServiceMonitor(addon),
		},
	}
	controllers.AddCommonLabels(serviceMonitorsInCluster.Items[0], addon)

	c.On("List", testutil.IsContext, mock.IsType(&monitoringv1.ServiceMonitorList{}), mock.Anything).
		Run(func(args mock.Arguments) {
			list := args.Get(1).(*monitoringv1.ServiceMonitorList)
			serviceMonitorsInCluster.DeepCopyInto(list)
		}).
		Return(nil)

	r := &monitoringFederationReconciler{
		client:                 c,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := context.Background()
	err := r.ensureDeletionOfUnwantedMonitoringFederation(ctx, addon)

	require.NoError(t, err)
	c.AssertExpectations(t)
}

// Test_monitoringFederationReconcilerName returns the expected value of monitoringFederationReconciler.
func TestMonitoringFederationReconcilerName(t *testing.T) {
	r := &monitoringFederationReconciler{}
	expected := MONITORING_FEDERATION_RECONCILER_NAME

	got := r.Name()

	assert.Equal(t, expected, got, "Expected Name() to return %q, but got %q", expected, got)
}

// TestMonitoringFederationReconcilerNameConstant checks if the constant name changes.
func TestMonitoringFederationReconcilerNameConstant(t *testing.T) {
	expected := "monitoringFederationReconciler"

	assert.Equal(t, expected, MONITORING_FEDERATION_RECONCILER_NAME, "Expected MONITORING_FEDERATION_RECONCILER_NAME to be %q, but got %q", expected, MONITORING_FEDERATION_RECONCILER_NAME)
}

type mockClient struct {
	client.Client
	listError error
}

func (m *mockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return m.listError
}

func TestMonitoringFederationReconciler_GetOwnedServiceMonitorsViaCommonLabels_Error(t *testing.T) {
	mockErr := fmt.Errorf("mocked list error")
	mockClient := &mockClient{
		listError: mockErr,
	}

	ctx := context.Background()
	addon := testutil.NewTestAddonWithMonitoringFederation()

	r := &monitoringFederationReconciler{}

	serviceMonitors, err := r.getOwnedServiceMonitorsViaCommonLabels(ctx, mockClient, addon)

	// Check the error
	expectedError := "could not list owned ServiceMonitors"
	assert.EqualError(t, err, expectedError, "Expected error message")

	// Check the serviceMonitors
	assert.Nil(t, serviceMonitors, "Expected serviceMonitors to be nil")
}
