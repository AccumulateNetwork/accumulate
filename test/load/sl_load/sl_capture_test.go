//go:build !testnet
// +build !testnet

package load_test

import (
	"testing"
)

// TestCaptureFees captures real fee data from network and saves it
func TestCaptureFees(t *testing.T) {
	// Skip if no devnet
	endpoint, err := FindDevnetEndpoint()
	if err != nil {
		t.Skip("No devnet available - skipping fee capture")
	}
	
	t.Logf("Found devnet at: %s", endpoint)
	
	// Capture the fee schedule
	err = CaptureFeeScheduleFromNetwork()
	if err != nil {
		t.Fatalf("Failed to capture fee schedule: %v", err)
	}
	
	t.Log("Successfully captured fee schedule from network")
}