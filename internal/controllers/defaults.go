package controllers

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultAddonInstanceHeartbeatTimeoutThresholdMultiplier int64 = 3
	DefaultOperatorGroupName                                      = "redhat-layered-product-og"
)

var DefaultAddonInstanceHeartbeatUpdatePeriod metav1.Duration = metav1.Duration{
	Duration: time.Second * 10,
}
