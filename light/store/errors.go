package store

import "errors"

var (
	// ErrLightBlockNotFound is returned when a store does not have the
	// requested header.
	ErrLightBLockNotFound = errors.New("light block not found")
)
