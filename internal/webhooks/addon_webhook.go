package webhooks

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	av1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

func NewAddonWebhook(validator admission.CustomValidator) *AddonWebhook {
	return &AddonWebhook{
		validator: validator,
	}
}

type AddonWebhook struct {
	validator admission.CustomValidator
}

func (w *AddonWebhook) SetupWithManager(mgr ctrl.Manager) error {
	return builder.WebhookManagedBy(mgr).
		For(&av1alpha1.Addon{}).
		WithValidator(w.validator).
		Complete()
}
