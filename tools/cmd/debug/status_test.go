package main

import (
	"bytes"
	"fmt"
	"log"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestNetworkStatusMainnet(t *testing.T) {
	// Set up a custom logger for debugging
	logPrefix := "[STATUS-TEST-DEBUG] "
	log.SetPrefix(logPrefix)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("Starting network status test with debug logging")
	
	// Enable debug logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	// Save and restore original stdout
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Set up command arguments with a longer timeout and skip consensus
	// Use a longer timeout to match our enhanced discovery implementation
	log.Printf("Setting up command with longer timeout (30s)")
	cmd.SetArgs([]string{"network", "status", "mainnet", "--timeout", "30s", "--skip-consensus"})

	// Start a timer to measure execution time
	start := time.Now()

	// Run the command
	log.Printf("Executing network status command")
	err := cmd.Execute()
	elapsed := time.Since(start)
	log.Printf("Command execution completed in %v", elapsed)
	w.Close()
	os.Stdout = origStdout

	// Read output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	fmt.Println("=== CLI Output ===")
	fmt.Printf("Command took %v to execute\n", elapsed)
	fmt.Print(output)
	if err != nil {
		t.Errorf("Command failed: %v", err)
	}
}
