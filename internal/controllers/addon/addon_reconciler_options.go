package addon

import (
	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AddonReconcilerOptions interface {
	ApplyToAddonReconciler(config *AddonReconciler)
	ApplyToControllerBuilder(b *builder.Builder)
}

type WithMonitoringStackReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func (w WithMonitoringStackReconciler) ApplyToAddonReconciler(config *AddonReconciler) {
	msReconciler := &monitoringStackReconciler{
		client: w.Client,
		scheme: w.Scheme,
	}
	config.subReconcilers = append(config.subReconcilers, msReconciler)
}

func (w WithMonitoringStackReconciler) ApplyToControllerBuilder(b *builder.Builder) {
	b.Owns(&obov1alpha1.MonitoringStack{})
}
