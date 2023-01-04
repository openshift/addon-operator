package runtimeoptions

const StatusReportingOption = "statusReporting"

func NewstatusReportingOption(opts ...controllerActionOnValue) Option {
	return newRuntimeOption(
		StatusReportingOption,
		opts...,
	)
}
