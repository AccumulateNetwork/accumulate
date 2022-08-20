package testing

import (
	"os"
	"runtime"
	"testing"
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
