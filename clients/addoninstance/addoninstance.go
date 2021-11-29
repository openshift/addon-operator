package addoninstance

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

func SetAddonInstanceCondition(ctx context.Context, cacheBackedKubeClient client.Client, condition metav1.Condition, addonName string) error {
	addonInstance, err := getAddonInstanceByAddon(ctx, cacheBackedKubeClient, addonName)
	if err != nil {
		return fmt.Errorf("failed to fetch AddonInstance by the addon '%s': %w", addonName, err)
	}
	if err := upsertAddonInstanceCondition(ctx, cacheBackedKubeClient, &addonInstance, condition); err != nil {
		return fmt.Errorf("failed to update the conditions of the AddonInstance resource in the namespace %s: %w", addonInstance.Namespace, err)
	}
	return nil
}

// SetupHeartReporter takes in a manager.Manager and hooks a periodic heartbeart reporter subroutine to it.
// This subroutine would report a heartbeat to the AddonInstance, corresponding to the provided addon, at a periodic interval suitable with respect to the AddonInstance's spc.heartbeatUpdatePeriod
// SetupHeartbeatReporter returns a channel (chan metav1.Condition) to which different kinds of heartbeats/metav1.Condition can be pushend/sent later on from various places across the codebase to make this heartbeat reporter subroutine start pushing those heartbeats instead.
// For example, say, in pkg/foo/bar.go, there's a place where an err is being captured and whenever that error occurs you want your addon start reporting an Unhealthy heartbeat until your addon becomes healthy, just do the following:
// .....
// unhealthyHeartbeatCondition := metav1.Condition{
// 	Type:    "addons.managed.openshift.io/Healthy",
// 	Status:  "False",
// 	Reason:  "ErrorX",
// 	Message: "Error X happening with the addon",
// }

// heartbeatCommunicatorChannel <- unhealthyHeartbeatCondition
// // from now on, automatically the addon would start reporting the above unhealthyHeartbeatCondition to the AddonInstance, until explicitly set otherwise
// .....
func SetupHeartbeatReporter(mgr manager.Manager, addonName string, handleAddonInstanceConfigurationChanges func(addonsv1alpha1.AddonInstanceSpec)) (heartbeatCommunicatorChannel chan metav1.Condition, err error) {
	// TODO(ykukreja): heartbeatCommunicatorChannel to be buffered channel instead, for better congestion control?
	// already some congestion control happening vi the timeout defined under utils.CommunicateHeartbeat(...)
	heartbeatCommunicatorChannel = make(chan metav1.Condition)
	defaultHealthyHeartbeatConditionToBeginWith := metav1.Condition{
		Type:    "addons.managed.openshift.io/Healthy",
		Status:  "True",
		Reason:  "AddonHealthy",
		Message: fmt.Sprintf("Addon '%s' is perfectly healthy", addonName),
	}

	heartbeatReporterFunction := func(ctx context.Context) error {
		// no significance of having heartbeatCommunicatorChannel open if this heartbeat reporter function is exited
		defer close(heartbeatCommunicatorChannel)

		// initialized with a healthy heartbeat condition corresponding to the addon
		currentHeartbeatCondition := defaultHealthyHeartbeatConditionToBeginWith

		currentAddonInstanceConfiguration, err := GetAddonInstanceConfiguration(ctx, mgr.GetClient(), addonName)
		if err != nil {
			return fmt.Errorf("failed to get the AddonInstance configuration corresponding to the Addon '%s': %w", addonName, err)
		}

		// Heartbeat reporter section: report a heartbeat at an interval ('currentAddonInstanceConfiguration.HeartbeatUpdatePeriod' seconds)
		for {
			select {
			case latestHeartbeatCondition := <-heartbeatCommunicatorChannel:
				currentHeartbeatCondition = latestHeartbeatCondition
				if err := SetAddonInstanceCondition(ctx, mgr.GetClient(), currentHeartbeatCondition, addonName); err != nil {
					mgr.GetLogger().Error(err, "error occurred while setting the condition", fmt.Sprintf("%+v", currentHeartbeatCondition))
				} // coz 'fallthrough' isn't allowed under select-case :'(
			case <-ctx.Done():
				return nil
			default:
				if err := SetAddonInstanceCondition(ctx, mgr.GetClient(), currentHeartbeatCondition, addonName); err != nil {
					mgr.GetLogger().Error(err, "error occurred while setting the condition", fmt.Sprintf("%+v", currentHeartbeatCondition))
				}
			}

			// checking latest addonInstance configuration and seeing if it differs with current AddonInstance configuration
			latestAddonInstanceConfiguration, err := GetAddonInstanceConfiguration(ctx, mgr.GetClient(), addonName)
			if err != nil {
				return fmt.Errorf("failed to get the AddonInstance configuration corresponding to the Addon '%s': %w", addonName, err)
			}
			if !reflect.DeepEqual(currentAddonInstanceConfiguration, latestAddonInstanceConfiguration) {
				currentAddonInstanceConfiguration = latestAddonInstanceConfiguration
				handleAddonInstanceConfigurationChanges(currentAddonInstanceConfiguration)
			}

			// waiting for heartbeat update period for executing the next iteration
			<-time.After(currentAddonInstanceConfiguration.HeartbeatUpdatePeriod.Duration)
		}
	}

	// coupling the heartbeat reporter function with the manager
	if err := mgr.Add(manager.RunnableFunc(heartbeatReporterFunction)); err != nil {
		return nil, err
	}
	return heartbeatCommunicatorChannel, nil
}

// TODO(ykukreja): make the timeout tweakable or dynamically self-configurable depending on the AddonInstance's heartbeat update period
// TODO(ykukreja): implement the following function to comply with buffered channels as well
func CommunicateHeartbeat(heartbeatCommunicatorChannelannel chan metav1.Condition, condition metav1.Condition, log logr.Logger) {
	go func() {
		select {
		case heartbeatCommunicatorChannelannel <- condition:
		case <-time.After(30 * time.Second):
			fmt.Println("heartbeat couldn't be sent due to it not being received by a receiver due to the channel being choked!") // just a placeholder message :P
		}
	}()
}

func GetAddonInstanceConfiguration(ctx context.Context, cacheBackedKubeClient client.Client, addonName string) (addonsv1alpha1.AddonInstanceSpec, error) {
	addonInstance, err := getAddonInstanceByAddon(ctx, cacheBackedKubeClient, addonName)
	if err != nil {
		return addonsv1alpha1.AddonInstanceSpec{}, fmt.Errorf("failed to fetch AddonInstance by the addon '%s': %w", addonName, err)
	}
	return addonInstance.Spec, nil
}
