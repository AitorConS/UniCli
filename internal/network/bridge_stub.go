//go:build !linux

package network

import "errors"

// BridgeConfig holds the parameters for a Linux bridge interface.
type BridgeConfig struct {
	Name string
	CIDR string
}

// CreateBridge is unavailable on non-Linux platforms.
func CreateBridge(_ BridgeConfig) error {
	return errors.New("bridge creation requires Linux")
}

// DestroyBridge is unavailable on non-Linux platforms.
func DestroyBridge(_ string) error {
	return errors.New("bridge creation requires Linux")
}

// AttachTAP is unavailable on non-Linux platforms.
func AttachTAP(_, _ string) error {
	return errors.New("TAP attachment requires Linux")
}

// DetachTAP is unavailable on non-Linux platforms.
func DetachTAP(_ string) error {
	return errors.New("TAP detachment requires Linux")
}