package addon

import corev1 "k8s.io/api/core/v1"

type NamespaceOpts func(*corev1.Namespace)

func WithNamespaceLabels(labels map[string]string) NamespaceOpts {
	return func(n *corev1.Namespace) {
		//nolint:staticcheck
		n.ObjectMeta.Labels = labels
	}
}

func WithNamespaceAnnotations(annotations map[string]string) NamespaceOpts {
	return func(n *corev1.Namespace) {
		//nolint:staticcheck
		n.ObjectMeta.Annotations = annotations
	}
}
