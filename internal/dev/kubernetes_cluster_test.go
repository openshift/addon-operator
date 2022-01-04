package dev

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aoapis "github.com/openshift/addon-operator/apis"
	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

func Test_checkObjectCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, aoapis.AddToScheme(scheme))

	tests := []struct {
		name   string
		object client.Object
		result bool
	}{
		{
			name: "structured deployment",
			object: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "test",
					Generation: 5,
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 5,
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentAvailable,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			result: true,
		},
		{
			name: "outdated structured deployment",
			object: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "test",
					Generation: 5,
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentAvailable,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			result: false,
		},
		{
			name: "outdated condition structured Addon",
			object: &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "test",
					Generation: 5,
				},
				Status: addonsv1alpha1.AddonStatus{
					ObservedGeneration: 5,
					Conditions: []metav1.Condition{
						{
							Type:               addonsv1alpha1.AddonOperatorAvailable,
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 3,
						},
					},
				},
			},
			result: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			done, err := checkObjectCondition(test.object,
				"Available", metav1.ConditionTrue, scheme)
			require.NoError(t, err)
			assert.Equal(t, test.result, done)
		})
	}
}
