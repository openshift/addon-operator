package addon

import (
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
	// append the config.subReconcilers with the instance of monitoringStackReconciler{} once its developed
	// this will make the AddonReconciler (corresponding to `config`) reconcile for MonitoringStack objects as well
	// msReconciler := &monitoringStackReconciler{
	// 	client: w.Client,
	// 	scheme: w.Scheme,
	// }
	// config.subReconcilers = append(config.subReconcilers, msReconciler)
}

func (w WithMonitoringStackReconciler) ApplyToControllerBuilder(b *builder.Builder) {
	// b.Owns(&monitoringstackv1alpha1.MonitoringStack{})
	// the above line would mark the addon-operator manager run the Control loop against any changes happening to the MonitoringStack CRs owned (as controller ref) by addon-operator
}
