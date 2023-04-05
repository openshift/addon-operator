package addon

import (
	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"
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

type WithPackageOperatorReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func (w WithPackageOperatorReconciler) ApplyToAddonReconciler(config *AddonReconciler) {
	poReconciler := &PackageOperatorReconciler{
		Client: w.Client,
		Scheme: w.Scheme,
	}
	config.subReconcilers = append(config.subReconcilers, poReconciler)
}

func (w WithPackageOperatorReconciler) ApplyToControllerBuilder(b *builder.Builder) {
	b.Owns(&pkov1alpha1.ClusterObjectTemplate{})
}
