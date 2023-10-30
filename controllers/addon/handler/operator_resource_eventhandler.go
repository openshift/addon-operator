package handler

import (
	"context"
	"sync"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OperatorResourceHandler struct {
	addonKeytoOperatorKeys map[client.ObjectKey][]client.ObjectKey
	operatorKeyToAddon     map[client.ObjectKey]client.ObjectKey
	mux                    sync.RWMutex
}

func NewOperatorResourceHandler() *OperatorResourceHandler {
	return &OperatorResourceHandler{
		addonKeytoOperatorKeys: map[client.ObjectKey][]client.ObjectKey{},
		operatorKeyToAddon:     map[client.ObjectKey]client.ObjectKey{},
	}
}

// Create is called in response to an create event.
func (h *OperatorResourceHandler) Create(_ context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	h.enqueueObject(evt.Object, q)
}

// Update is called in response to an update event.
func (h *OperatorResourceHandler) Update(_ context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	h.enqueueObject(evt.ObjectNew, q)
}

// Delete is called in response to a delete event.
func (h *OperatorResourceHandler) Delete(_ context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	h.enqueueObject(evt.Object, q)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *OperatorResourceHandler) Generic(_ context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	h.enqueueObject(evt.Object, q)
}

func (h *OperatorResourceHandler) enqueueObject(obj client.Object, q workqueue.RateLimitingInterface) {
	h.mux.RLock()
	defer h.mux.RUnlock()

	operatorKey := client.ObjectKeyFromObject(obj)
	addonKey, ok := h.operatorKeyToAddon[operatorKey]
	if !ok {
		return
	}

	q.Add(reconcile.Request{NamespacedName: addonKey})
}

// Free removes all event mappings associated with the given Addon.
func (h *OperatorResourceHandler) Free(addon *addonsv1alpha1.Addon) {
	h.mux.Lock()
	defer h.mux.Unlock()

	addonKey := client.ObjectKeyFromObject(addon)
	for _, operatorKey := range h.addonKeytoOperatorKeys[addonKey] {
		delete(h.operatorKeyToAddon, operatorKey)
	}
	delete(h.addonKeytoOperatorKeys, addonKey)
}

func (h *OperatorResourceHandler) UpdateMap(addon *addonsv1alpha1.Addon, operatorKey client.ObjectKey) (changed bool) {
	h.mux.Lock()
	defer h.mux.Unlock()

	addonKey := client.ObjectKeyFromObject(addon)
	if !isKeyPresent(h.addonKeytoOperatorKeys[addonKey], operatorKey) {
		h.addonKeytoOperatorKeys[addonKey] = append(h.addonKeytoOperatorKeys[addonKey], operatorKey)
		changed = true
	}

	aKey, ok := h.operatorKeyToAddon[operatorKey]
	if !ok || aKey != addonKey {
		h.operatorKeyToAddon[operatorKey] = addonKey
		changed = true
	}

	return changed
}

func isKeyPresent(exiting []client.ObjectKey, key client.ObjectKey) bool {
	for _, k := range exiting {
		if k == key {
			return true
		}
	}
	return false
}
