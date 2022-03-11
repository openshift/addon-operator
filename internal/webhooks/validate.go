package webhooks

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

var (
	errSpecInstallTypeInvalid             = errors.New("invalid Addon .spec.install.type")
	errSpecInstallOwnNamespaceRequired    = errors.New(".spec.install.olmOwnNamespace is required when .spec.install.type = OLMOwnNamespace")
	errSpecInstallAllNamespacesRequired   = errors.New(".spec.install.olmAllNamespaces is required when .spec.install.type = OLMAllNamespaces")
	errSpecInstallConfigMutuallyExclusive = errors.New(".spec.install.olmAllNamespaces is mutually exclusive with .spec.install.olmOwnNamespace")
)

func validateAddon(addon *addonsv1alpha1.Addon) error {
	if err := validateInstallSpec(addon.Spec.Install); err != nil {
		return err
	}
	if err := validateSecretPropagation(addon); err != nil {
		return err
	}
	return nil
}

func validateSecretPropagation(addon *addonsv1alpha1.Addon) error {
	var pullSecretName string
	switch addon.Spec.Install.Type {
	case addonsv1alpha1.OLMAllNamespaces:
		pullSecretName = addon.Spec.Install.OLMAllNamespaces.PullSecretName
	case addonsv1alpha1.OLMOwnNamespace:
		pullSecretName = addon.Spec.Install.OLMOwnNamespace.PullSecretName
	}

	if len(pullSecretName) == 0 || addon.Spec.SecretPropagation == nil {
		return nil
	}

	for _, secret := range addon.Spec.SecretPropagation.Secrets {
		if secret.DestinationSecret.Name == pullSecretName {
			// pullSecretName is part of the secret list!
			return nil
		}
	}

	// we have not found pullSecretName in the secret propagation list.
	return fmt.Errorf("pullSecretName %q not found as destination in secretPropagation", pullSecretName)
}

func validateInstallSpec(addonSpecInstall addonsv1alpha1.AddonInstallSpec) error {
	if addonSpecInstall.OLMAllNamespaces != nil &&
		addonSpecInstall.OLMOwnNamespace != nil {
		return errSpecInstallConfigMutuallyExclusive
	}

	switch addonSpecInstall.Type {
	case addonsv1alpha1.OLMOwnNamespace:
		if addonSpecInstall.OLMOwnNamespace == nil {
			// missing configuration
			return errSpecInstallOwnNamespaceRequired
		}

		return nil

	case addonsv1alpha1.OLMAllNamespaces:
		if addonSpecInstall.OLMAllNamespaces == nil {
			// missing configuration
			return errSpecInstallAllNamespacesRequired
		}

		return nil

	default:
		// Unsupported Install Type
		// This should never happen, unless the schema validation is wrong.
		// The .install.type property is set to only allow known enum values.
		return errSpecInstallTypeInvalid
	}
}

var (
	errInstallTypeImmutable = errors.New(".spec.install.type is immutable")
	errInstallImmutable     = errors.New(".spec.install is immutable, except for .catalogSourceImage")
)

func validateAddonImmutability(addon, oldAddon *addonsv1alpha1.Addon) error {
	if addon.Spec.Install.Type != oldAddon.Spec.Install.Type {
		return errInstallTypeImmutable
	}

	// empty fields that we don't want to compare
	oldSpecInstall := oldAddon.Spec.Install.DeepCopy()
	if oldSpecInstall.OLMAllNamespaces != nil {
		oldSpecInstall.OLMAllNamespaces.CatalogSourceImage = ""
		oldSpecInstall.OLMAllNamespaces.Config = nil
		oldSpecInstall.OLMAllNamespaces.PullSecretName = ""
	}
	if oldSpecInstall.OLMOwnNamespace != nil {
		oldSpecInstall.OLMOwnNamespace.CatalogSourceImage = ""
		oldSpecInstall.OLMOwnNamespace.Config = nil
		oldSpecInstall.OLMOwnNamespace.PullSecretName = ""
	}

	specInstall := addon.Spec.Install.DeepCopy()
	if specInstall.OLMAllNamespaces != nil {
		specInstall.OLMAllNamespaces.CatalogSourceImage = ""
		specInstall.OLMAllNamespaces.Config = nil
		specInstall.OLMAllNamespaces.PullSecretName = ""
	}
	if specInstall.OLMOwnNamespace != nil {
		specInstall.OLMOwnNamespace.CatalogSourceImage = ""
		specInstall.OLMOwnNamespace.Config = nil
		specInstall.OLMOwnNamespace.PullSecretName = ""
	}

	// Do semantic DeepEqual instead of reflect.DeepEqual
	if !equality.Semantic.DeepEqual(oldSpecInstall, specInstall) {
		return errInstallImmutable
	}
	return nil
}
