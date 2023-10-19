package webhooks

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	av1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

func NewAddonWebhooks(validator admission.CustomValidator) *AddonWebhooks {
	return &AddonWebhooks{
		validator: validator,
	}
}

type AddonWebhooks struct {
	validator admission.CustomValidator
}

func (w *AddonWebhooks) SetupWithManager(mgr ctrl.Manager) error {
	return builder.WebhookManagedBy(mgr).
		For(&av1alpha1.Addon{}).
		WithValidator(w.validator).
		Complete()
}
