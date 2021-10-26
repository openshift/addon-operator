package metrics

import (
	"sync"
	"time"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

// helper type to track phase and installation time
type addonState struct {
	Phase       string
	CreatedAt   time.Time
	InstalledAt time.Time
}

// maps addon UIDs to their state
var addonsStateMapping = make(map[string]addonState)
var lock sync.RWMutex

// UpdateAddonMetrics - helper function to update addon metrics
func UpdateAddonMetrics(addon *addonsv1alpha1.Addon, currPhaseRaw addonsv1alpha1.AddonPhase) {
	// Update the map in a concurrent safe manner
	lock.Lock()
	defer lock.Unlock()

	uid := string(addon.UID)
	currPhase := string(currPhaseRaw)
	now := time.Now().UTC()

	oldState, ok := addonsStateMapping[uid]
	if !ok {
		// new addon
		addonsStateMapping[uid] = addonState{
			Phase:     currPhase,
			CreatedAt: now,
		}
		AddonsPerPhaseTotal.WithLabelValues(addon.Name, currPhase).Inc()
		AddonsInstallationTotal.WithLabelValues(addon.Name).Inc()
	} else if oldState.Phase == currPhase {
		// nothing to do
		return
	} else {
		// old addon
		currState := addonState{
			Phase:       currPhase,
			CreatedAt:   oldState.CreatedAt,
			InstalledAt: oldState.InstalledAt,
		}
		AddonsPerPhaseTotal.WithLabelValues(addon.Name, currPhase).Inc()
		AddonsPerPhaseTotal.WithLabelValues(addon.Name, oldState.Phase).Dec()

		// detect successful installation
		if currState.InstalledAt.IsZero() && currPhaseRaw == addonsv1alpha1.PhaseReady {
			currState.InstalledAt = now
			installTime := now.Sub(currState.CreatedAt).Seconds()
			AddonsInstallationSuccessTimeSeconds.WithLabelValues(addon.Name).Observe(installTime)
		}

		// update mapping
		addonsStateMapping[uid] = currState
	}
}
