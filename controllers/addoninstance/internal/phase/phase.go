package phase

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	av1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
)

type Request struct {
	Instance av1alpha1.AddonInstance
}

func Success(conditions ...metav1.Condition) Result {
	return Result{
		Conditions: conditions,
	}
}

func Error(err error) Result {
	return Result{
		err: err,
	}
}

type Result struct {
	Conditions []metav1.Condition
	err        error
}

func (r *Result) Error() error {
	return r.err
}
