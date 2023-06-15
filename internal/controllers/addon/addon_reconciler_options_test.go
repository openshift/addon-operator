package addon

import (
	"context"
	"testing"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Mock implementation of addonReconciler for testing
type mockReconciler struct{}

func (r *mockReconciler) Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	// Mock implementation
	return ctrl.Result{}, nil
}

func (r *mockReconciler) Name() string {
	return "MockReconciler"
}
func TestWithMonitoringStackReconciler_ApplyToAddonReconciler(t *testing.T) {
	c := testutil.NewClient()

	type fields struct {
		Client client.Client
		Scheme *runtime.Scheme
	}
	type args struct {
		config *AddonReconciler
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		initial int
	}{
		{
			name: "Adds monitoringStackReconciler to empty subReconcilers",
			fields: fields{
				Client: c,
				Scheme: testutil.NewTestSchemeWithAddonsv1alpha1AndMsov1alpha1(),
			},
			args: args{
				config: &AddonReconciler{
					subReconcilers: []addonReconciler{},
				},
			},
			initial: 0,
		},
		{
			name: "Adds monitoringStackReconciler to non-empty subReconcilers",
			fields: fields{
				Client: c,
				Scheme: testutil.NewTestSchemeWithAddonsv1alpha1AndMsov1alpha1(),
			},
			args: args{
				config: &AddonReconciler{
					subReconcilers: []addonReconciler{
						&mockReconciler{},
					},
				},
			},
			initial: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := WithMonitoringStackReconciler{
				Client: tt.fields.Client,
				Scheme: tt.fields.Scheme,
			}
			tt.args.config.subReconcilers = make([]addonReconciler, tt.initial)
			w.ApplyToAddonReconciler(tt.args.config)

			// Check that the length of the subReconciler slice has increased
			assert.Len(t, tt.args.config.subReconcilers, tt.initial+1)
			assert.IsType(t, &monitoringStackReconciler{}, tt.args.config.subReconcilers[tt.initial])
		})
	}
}
