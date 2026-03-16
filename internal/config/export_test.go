package config

import "runtime"

// SetGOOS overrides the OS identifier used by DefaultPath (for testing).
func SetGOOS(os string) { goos = os }

// ResetGOOS restores the OS identifier to the real runtime value.
func ResetGOOS() { goos = runtime.GOOS }
