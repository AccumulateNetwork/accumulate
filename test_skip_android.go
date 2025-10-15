//go:build android
// +build android

// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulate

import (
	"os"
	"runtime"
	"testing"
)

// SkipOnAndroid skips tests that are known to be problematic on Android/Termux
func SkipOnAndroid(t *testing.T, reason string) {
	if runtime.GOOS == "android" || isTermux() {
		t.Skipf("Skipping on Android/Termux: %s", reason)
	}
}

// isTermux detects if we're running in Termux environment
func isTermux() bool {
	// Check for Termux-specific environment variables
	if os.Getenv("TERMUX_VERSION") != "" {
		return true
	}
	if os.Getenv("PREFIX") == "/data/data/com.termux/files/usr" {
		return true
	}
	// Check if we're in the Termux directory structure
	if _, err := os.Stat("/data/data/com.termux/files/usr"); err == nil {
		return true
	}
	return false
}
