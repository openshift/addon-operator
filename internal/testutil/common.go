package testutil

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"golang.org/x/exp/slices"

	"github.com/stretchr/testify/assert"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

// NewStatusError returns an error of type `StatusError `
func NewStatusError(msg string) *k8sApiErrors.StatusError {
	return &k8sApiErrors.StatusError{
		ErrStatus: metav1.Status{
			Status: "Failure",
			Message: fmt.Sprintf("%s %s",
				"admission webhook \"vaddons.managed.openshift.io\" denied the request:",
				msg),
			Reason: metav1.StatusReason(msg),
			Code:   403,
		},
	}
}

// NewAddonWithInstallSpec returns an Addon object with the specified InstallSpec
func NewAddonWithInstallSpec(installSpec addonsv1alpha1.AddonInstallSpec,
	addonName string) *addonsv1alpha1.Addon {
	return &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: addonName,
		},
		Spec: addonsv1alpha1.AddonSpec{
			DisplayName: "An example addon",
			Namespaces: []addonsv1alpha1.AddonNamespace{
				{Name: "reference-addon"},
			},
			Install: installSpec,
		},
	}
}

func IsWebhookServerEnabled() bool {
	value, exists := os.LookupEnv("ENABLE_WEBHOOK")
	return exists && value != "false"
}

func IsApiMockEnabled() bool {
	value, exists := os.LookupEnv("ENABLE_API_MOCK")
	return exists && value != "false"
}

func AssertConditionsMatch(t *testing.T, condsA []metav1.Condition, condsB []metav1.Condition) {
	a := make([]metav1.Condition, 0, len(condsA))

	for _, c := range condsA {
		a = append(a, stripTransients(c))
	}

	b := make([]metav1.Condition, 0, len(condsA))

	for _, c := range condsB {
		b = append(b, stripTransients(c))
	}

	assert.ElementsMatch(t, a, b)
}

func stripTransients(cond metav1.Condition) metav1.Condition {
	return metav1.Condition{
		Type:    cond.Type,
		Status:  cond.Status,
		Reason:  cond.Reason,
		Message: cond.Message,
	}
}

func IsEnabledOnTestEnv(featureFlagIdentifier string) bool {
	commaSeparatedFeatureFlags, ok := os.LookupEnv("FEATURE_TOGGLES")
	if !ok {
		return false
	}
	return slices.Contains(strings.Split(commaSeparatedFeatureFlags, ","), featureFlagIdentifier)
}
