package runtimeoptions

const GlobalPauseOption = "globalPause"

func NewGlobalPauseOption(opts ...controllerActionOnValue) Option {
	return newRuntimeOption(
		GlobalPauseOption,
		opts...,
	)
}
