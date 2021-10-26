package integration

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

// Default Interval in which to recheck wait conditions.
const defaultWaitPollInterval = time.Second

// WaitToBeGone blocks until the given object is gone from the kubernetes API server.
func WaitToBeGone(t *testing.T, timeout time.Duration, object client.Object) error {
	gvk, err := apiutil.GVKForObject(object, Scheme)
	if err != nil {
		return err
	}

	key := client.ObjectKeyFromObject(object)
	t.Logf("waiting %s for %s %s to be gone...",
		timeout, gvk, key)

	ctx := context.Background()
	return wait.PollImmediate(defaultWaitPollInterval, timeout, func() (done bool, err error) {
		err = Client.Get(ctx, key, object)

		if errors.IsNotFound(err) {
			return true, nil
		}

		if err != nil {
			t.Logf("error waiting for %s %s to be gone: %v",
				object.GetObjectKind().GroupVersionKind().Kind, key, err)
		}
		return false, nil
	})
}

// Wait that something happens with an object.
func WaitForObject(
	t *testing.T, timeout time.Duration,
	object client.Object, reason string,
	checkFn func(obj client.Object) (done bool, err error),
) error {
	gvk, err := apiutil.GVKForObject(object, Scheme)
	if err != nil {
		return err
	}

	key := client.ObjectKeyFromObject(object)
	t.Logf("waiting %s on %s %s %s...",
		timeout, gvk, key, reason)

	ctx := context.Background()
	return wait.PollImmediate(time.Second, timeout, func() (done bool, err error) {
		err = Client.Get(ctx, client.ObjectKeyFromObject(object), object)
		if err != nil {
			return false, nil
		}

		return checkFn(object)
	})
}

func WaitForAddonToBeAvailable(t *testing.T, timeout time.Duration, addon client.Object) error {
	reason := "to be Available"
	checkFn := func(obj client.Object) (done bool, err error) {
		a := obj.(*addonsv1alpha1.Addon)
		return meta.IsStatusConditionTrue(
			a.Status.Conditions, addonsv1alpha1.Available), nil
	}
	return WaitForObject(t, timeout, addon, reason, checkFn)
}
