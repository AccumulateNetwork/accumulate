package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// TestScReconstruct tests the snapshot reconstruction functionality
// using a real snapshot file
func TestScReconstruct(t *testing.T) {
	// Path to the test snapshot file
	sourceSnapshot := "/home/paul/work/acc1/dn.snap"

	// Create a temporary directory for the test output
	tempDir, err := os.MkdirTemp("", "sc-reconstruct-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir) // Clean up after the test

	// Path to destination snapshot
	destSnapshot := filepath.Join(tempDir, "reconstructed.snap")

	// Create a mock cobra command to simulate CLI usage
	cmd := &cobra.Command{}

	// Set up arguments as they would be passed from the CLI
	// First argument is the output path, second is the input snapshot
	args := []string{destSnapshot, sourceSnapshot}

	err = sc_Run(cmd, args)
	if err != nil {
		t.Fatalf("Reconstruction failed: %v", err)
	}

	// Verify the output file exists and has content
	if _, err := os.Stat(destSnapshot); os.IsNotExist(err) {
		t.Errorf("Output file was not created at %s", destSnapshot)
	} else {
		// Get file size
		fileInfo, err := os.Stat(destSnapshot)
		if err != nil {
			t.Errorf("Failed to get output file info: %v", err)
		} else {
			fmt.Printf("Successfully created reconstructed snapshot at %s (%d bytes)\n",
				destSnapshot, fileInfo.Size())

			// Basic validation: file should not be empty
			if fileInfo.Size() == 0 {
				t.Errorf("Reconstructed file is empty")
			}
		}
	}

	fmt.Println("Test completed successfully")
}
