package common

import "errors"

var (
	// ErrNotOwnedByUs is returned when a reconciled child object already
	// exists and is not owned by the current controller/addon
	ErrNotOwnedByUs = errors.New("object is not owned by us")
)
