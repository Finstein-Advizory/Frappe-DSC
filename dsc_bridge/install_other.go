//go:build !windows && !darwin && !linux

package main

import "fmt"

// PostInstall is a no-op stub for platforms without a dedicated installer
// (BSD, etc.). Windows, macOS, and Linux each have their own install_*.go with
// real autostart / firewall / certificate-trust logic.
func PostInstall() error {
	return fmt.Errorf("--post-install is not implemented on this platform")
}

// PreUninstall mirrors PostInstall.
func PreUninstall() error {
	return fmt.Errorf("--pre-uninstall is not implemented on this platform")
}

// EnsureUserTrust is a no-op here; per-user browser trust is only automated on
// Linux (NSS DBs). See install_linux.go.
func EnsureUserTrust() {}
