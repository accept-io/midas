package dispatch

import (
	"errors"
	"fmt"
)

// ErrNilArg is returned by NewDispatcher when a required argument is nil.
var ErrNilArg = errors.New("dispatch: required argument is nil")

// ErrInvalidConfig is returned by NewDispatcher when a configuration value
// violates a constraint.
var ErrInvalidConfig = errors.New("dispatch: invalid configuration")

func errNilArg(name string) error {
	return fmt.Errorf("%w: %s", ErrNilArg, name)
}

func errInvalidCfg(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, msg)
}
