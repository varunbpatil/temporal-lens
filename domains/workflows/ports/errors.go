package ports

import "errors"

// Port errors represent adapter-level conditions that all implementations
// must return consistently. The domain service uses these to map adapter
// errors to the appropriate domain errors defined in domains/workflows/errors.go.
var (
	// ErrIndexAlreadyExists is returned when attempting to create an index that already exists.
	ErrIndexAlreadyExists = errors.New("index already exists")
)
