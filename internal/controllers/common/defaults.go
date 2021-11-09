package common

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AddonInstance default consts
const (
	DefaultAddonInstanceHeartbeatTimeoutThresholdMultiplier int64 = 3
)

// AddonInstance default vars
var (
	DefaultAddonInstanceHeartbeatUpdatePeriod metav1.Duration = metav1.Duration{
		Duration: time.Second * 10,
	}
)

// Addon default consts
const (
	DefaultRetryAfterTime = 10 * time.Second
)

// AddonOperator default consts
const (
	DefaultAddonOperatorRequeueTime = time.Minute
)
