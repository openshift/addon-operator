package addon

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
)

// Ensures the presence of a NetworkPolicy allowing ingress to the Addon's CatalogSources
func (r *olmReconciler) ensureCatalogSourcesNetworkPolicy(ctx context.Context, addon *addonsv1alpha1.Addon) (requeueResult, error) {
	desired, err := r.desiredCatalogSourcesNetworkPolicy(ctx, addon)
	if err != nil {
		if errors.Is(err, errInstallConfigParseFailure) {
			return resultStop, nil
		}

		return resultNil, fmt.Errorf("building desired NetworkPolicy: %w", err)
	}

	actual, err := r.actualCatalogSourcesNetworkPolicy(ctx, addon)
	if err != nil {
		if errors.Is(err, errInstallConfigParseFailure) {
			return resultStop, nil
		}
		if k8serrors.IsNotFound(err) {
			return resultNil, r.client.Create(ctx, desired)
		}

		return resultNil, fmt.Errorf("retrieving actual NetworkPolicy: %w", err)
	}

	currentLabels := labels.Set(actual.Labels)
	newLabels := labels.Merge(currentLabels, labels.Set(desired.Labels))

	ownedByAddon := controllers.HasSameController(actual, desired)
	specChanged := !equality.Semantic.DeepEqual(actual.Spec, desired.Spec)
	labelsChanged := !labels.Equals(currentLabels, newLabels)

	if ownedByAddon && !specChanged && !labelsChanged {
		return resultNil, nil
	}

	actual.OwnerReferences = desired.OwnerReferences
	actual.Spec = desired.Spec
	actual.Labels = newLabels

	return resultNil, r.client.Update(ctx, actual)
}

var errInstallConfigParseFailure = errors.New("failed to parse addon install config")

func (r *olmReconciler) desiredCatalogSourcesNetworkPolicy(ctx context.Context, addon *addonsv1alpha1.Addon) (*networkingv1.NetworkPolicy, error) {
	log := controllers.LoggerFromContext(ctx)

	installConfig, stop := parseAddonInstallConfig(log, addon)
	if stop {
		return nil, errInstallConfigParseFailure
	}

	catalogSourceNames := []string{getPrimaryCatalogSourceName(addon)}

	for _, acs := range installConfig.AdditionalCatalogSources {
		catalogSourceNames = append(catalogSourceNames, acs.Name)
	}

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getCatalogSourceNetworkPolicyName(addon),
			Namespace: installConfig.Namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "olm.catalogSource",
						Operator: metav1.LabelSelectorOpIn,
						Values:   catalogSourceNames,
					},
				},
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: corev1ProtocolPtr(corev1.ProtocolTCP),
							Port:     intOrStringPtr(intstr.FromInt(50051)),
						},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		},
	}

	controllers.AddCommonLabels(np, addon)
	controllers.AddCommonAnnotations(np, addon)
	if err := controllerutil.SetControllerReference(addon, np, r.scheme); err != nil {
		return nil, fmt.Errorf("setting controller reference: %w", err)
	}

	return np, nil
}

func (r *olmReconciler) actualCatalogSourcesNetworkPolicy(ctx context.Context, addon *addonsv1alpha1.Addon) (*networkingv1.NetworkPolicy, error) {
	log := controllers.LoggerFromContext(ctx)

	installConfig, stop := parseAddonInstallConfig(log, addon)
	if stop {
		return nil, errInstallConfigParseFailure
	}

	key := client.ObjectKey{
		Name:      getCatalogSourceNetworkPolicyName(addon),
		Namespace: installConfig.Namespace,
	}

	var networkPolicy networkingv1.NetworkPolicy

	if err := r.client.Get(ctx, key, &networkPolicy); err != nil {
		return nil, fmt.Errorf("getting NetworkPolicy: %w", err)
	}

	return &networkPolicy, nil
}
