package addonoperator

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/mt-sre/client"
	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAddonOperatorReconciler_handleAddonOperatorCreation(t *testing.T) {
	type fields struct {
		Client              client.Client
		UncachedClient      client.Client
		Log                 logr.Logger
		Scheme              *runtime.Scheme
		GlobalPauseManager  globalPauseManager
		OCMClientManager    ocmClientManager
		Recorder            *metrics.Recorder
		ClusterExternalID   string
		FeatureTogglesState []string
	}
	type args struct {
		ctx context.Context
		log logr.Logger
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &AddonOperatorReconciler{
				Client:              tt.fields.Client,
				UncachedClient:      tt.fields.UncachedClient,
				Log:                 tt.fields.Log,
				Scheme:              tt.fields.Scheme,
				GlobalPauseManager:  tt.fields.GlobalPauseManager,
				OCMClientManager:    tt.fields.OCMClientManager,
				Recorder:            tt.fields.Recorder,
				ClusterExternalID:   tt.fields.ClusterExternalID,
				FeatureTogglesState: tt.fields.FeatureTogglesState,
			}
			if err := r.handleAddonOperatorCreation(tt.args.ctx, tt.args.log); (err != nil) != tt.wantErr {
				t.Errorf("AddonOperatorReconciler.handleAddonOperatorCreation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAddonOperatorReconciler_reportAddonOperatorReadinessStatus(t *testing.T) {
	type fields struct {
		Client              client.Client
		UncachedClient      client.Client
		Log                 logr.Logger
		Scheme              *runtime.Scheme
		GlobalPauseManager  globalPauseManager
		OCMClientManager    ocmClientManager
		Recorder            *metrics.Recorder
		ClusterExternalID   string
		FeatureTogglesState []string
	}
	type args struct {
		ctx           context.Context
		addonOperator *addonsv1alpha1.AddonOperator
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &AddonOperatorReconciler{
				Client:              tt.fields.Client,
				UncachedClient:      tt.fields.UncachedClient,
				Log:                 tt.fields.Log,
				Scheme:              tt.fields.Scheme,
				GlobalPauseManager:  tt.fields.GlobalPauseManager,
				OCMClientManager:    tt.fields.OCMClientManager,
				Recorder:            tt.fields.Recorder,
				ClusterExternalID:   tt.fields.ClusterExternalID,
				FeatureTogglesState: tt.fields.FeatureTogglesState,
			}
			if err := r.reportAddonOperatorReadinessStatus(tt.args.ctx, tt.args.addonOperator); (err != nil) != tt.wantErr {
				t.Errorf("AddonOperatorReconciler.reportAddonOperatorReadinessStatus() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
