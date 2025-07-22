package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run monitor.go root-hash-monitor.go <network>")
		fmt.Println("Networks: local, testnet, beta, canary, mainnet, mainnet-ssl")
		fmt.Println()
		fmt.Println("Example: go run monitor.go root-hash-monitor.go mainnet")
		os.Exit(1)
	}

	network := os.Args[1]
	
	// Create root hash monitor
	monitor, err := NewRootHashMonitor(network)
	if err != nil {
		log.Fatalf("Failed to create root hash monitor: %v", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down...")
		cancel()
	}()

	// Start monitoring
	fmt.Printf("Starting Accumulate root hash monitor for network: %s\n", network)
	if err := monitor.MonitorRootHash(ctx); err != nil && err != context.Canceled {
		log.Fatalf("Root hash monitoring failed: %v", err)
	}
}
