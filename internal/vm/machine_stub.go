//go:build !linux

package vm

import (
	"context"
	"errors"
)

// ErrLinuxOnly is returned on non-Linux platforms.
var ErrLinuxOnly = errors.New("firecracker requires Linux with /dev/kvm")

// Machine is a placeholder on non-Linux platforms.
type Machine struct{}

func NewMachine(_ context.Context, _ RunOptions, _ bool) (*Machine, RuntimeConfig, error) {
	return nil, RuntimeConfig{}, ErrLinuxOnly
}

func Start(_ context.Context, _ *Machine) error      { return ErrLinuxOnly }
func StopForce(_ *Machine) error                     { return ErrLinuxOnly }
func ShutdownGuest(_ context.Context, _ *Machine) error { return ErrLinuxOnly }
func Wait(_ context.Context, _ *Machine) error       { return ErrLinuxOnly }
func PID(_ *Machine) (int, error)                    { return 0, ErrLinuxOnly }
