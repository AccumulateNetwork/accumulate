package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	fmt.Println("Running: debug sequence mainnet")
	fmt.Println("===============================")
	fmt.Println()

	cmd := exec.Command("./debug", "sequence", "mainnet", "--verbose")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	// Start the command
	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to start command: %v\n", err)
		os.Exit(1)
	}
	
	// Wait for it to complete or timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	
	select {
	case err := <-done:
		if err != nil {
			fmt.Printf("\nCommand failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\nCommand completed successfully")
	case <-time.After(60 * time.Second):
		fmt.Println("\n\nCommand timed out after 60 seconds")
		fmt.Println("This likely indicates network connectivity issues")
		fmt.Println("The mainnet P2P nodes may be unreachable from this location")
		cmd.Process.Kill()
		os.Exit(124)
	}
}