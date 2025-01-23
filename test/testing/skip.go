// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testing

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// SkipLong skips a long test when running in -short mode.
func SkipLong(t testing.TB) {
	if testing.Short() {
		t.Skip("Skipping test: running with -short")
	}
}

// SkipCI skips a test when running in CI.
func SkipCI(t testing.TB, reason string) {
	if os.Getenv("CI") == "true" {
		t.Skipf("Skipping test: running CI: %s", reason)
	}
}

// SkipPlatform skips a test when on a specific GOOS.
func SkipPlatform(t testing.TB, goos, reason string) {
	if runtime.GOOS == goos {
		t.Skipf("Skipping test: running on %s: %s", goos, reason)
	}
}

// SkipPlatformCI skips a test when running in CI on a specific GOOS.
func SkipPlatformCI(t testing.TB, goos, reason string) {
	if runtime.GOOS == goos && os.Getenv("CI") == "true" {
		t.Skipf("Skipping test: running CI on %s: %s", goos, reason)
	}
}

// SkipWithoutTool skips a test if the given tool is not found on the path,
// unless the test is running in CI, in which case it fails if the tool is not
// present.
func SkipWithoutTool(t testing.TB, toolName string) {
	_, err := exec.LookPath(toolName)
	if err != nil {
		if !errors.Is(err, exec.ErrNotFound) || os.Getenv("CI") == "true" {
			require.NoError(t, err)
		}
		t.Skip("Cannot locate node binary")
	}
}
