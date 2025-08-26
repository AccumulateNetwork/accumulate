// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// network_monitor displays real-time network status and metrics
// for Accumulate networks with automatic refresh.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

func main() {
	var (
		network  = flag.String("network", "mainnet", "Network to connect to (mainnet, testnet, local)")
		interval = flag.Duration("interval", 5*time.Second, "Refresh interval")
		once     = flag.Bool("once", false, "Run once and exit")
		web      = flag.Bool("web", false, "Launch web UI")
		port     = flag.Int("port", 8080, "Web server port")
	)
	flag.Parse()

	// Create client
	c, err := createClient(*network)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	if *web {
		// Launch web server
		server := NewWebServer(c, *port)
		url := fmt.Sprintf("http://localhost:%d", *port)
		fmt.Printf("🌐 Starting web UI at %s\n", url)
		fmt.Println("Press Ctrl+C to stop")
		
		// Try to open browser automatically
		go func() {
			time.Sleep(1 * time.Second) // Give server time to start
			openBrowser(url)
		}()
		
		if err := server.Start(); err != nil {
			log.Fatalf("Failed to start web server: %v", err)
		}
	} else {
		// CLI mode
		monitor := &NetworkMonitor{
			client:   c,
			interval: *interval,
		}

		// Handle interrupt
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			fmt.Println("\n\nShutting down...")
			cancel()
		}()

		if *once {
			// Run once
			if err := monitor.Display(ctx); err != nil {
				log.Fatalf("Failed to display status: %v", err)
			}
		} else {
			// Run continuously
			monitor.Run(ctx)
		}
	}
}

func createClient(network string) (*client.Client, error) {
	switch network {
	case "mainnet":
		return client.NewMainnet()
	case "testnet":
		return client.NewTestnet()
	case "local":
		endpoint := os.Getenv("ACCUMULATE_ENDPOINT")
		if endpoint == "" {
			endpoint = "http://localhost:8080/v3"
		}
		return client.NewLocal(endpoint)
	default:
		return nil, fmt.Errorf("unknown network: %s", network)
	}
}

type NetworkMonitor struct {
	client   *client.Client
	interval time.Duration
}

func (m *NetworkMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	// Display immediately
	if err := m.Display(ctx); err != nil {
		log.Printf("Error: %v", err)
	}

	// Then refresh on interval
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Clear screen (ANSI escape code)
			fmt.Print("\033[H\033[2J")
			if err := m.Display(ctx); err != nil {
				log.Printf("Error: %v", err)
			}
		}
	}
}

func (m *NetworkMonitor) Display(ctx context.Context) error {
	// Create a context with timeout for each request
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Gather all information
	nodeInfo, _ := m.client.GetNodeInfo(ctx)
	networkStatus, _ := m.client.GetNetworkStatus(ctx)
	
	// Display header
	fmt.Println("=" + strings.Repeat("=", 78))
	fmt.Println("                     🌐 ACCUMULATE NETWORK MONITOR")
	fmt.Println("=" + strings.Repeat("=", 78))
	fmt.Printf("Last Update: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println()

	// Display Node Information
	if nodeInfo != nil {
		fmt.Println("📡 NODE INFORMATION")
		fmt.Println("-" + strings.Repeat("-", 78))
		fmt.Printf("  Network:    %s\n", nodeInfo.Network)
		fmt.Printf("  Peer ID:    %s\n", nodeInfo.PeerID)
		fmt.Printf("  Version:    %s\n", nodeInfo.Version)
		fmt.Printf("  Commit:     %s\n", nodeInfo.Commit[:8])
		
		// Count services by type
		services := make(map[string]int)
		for _, svc := range nodeInfo.Services {
			services[svc.Type.String()]++
		}
		fmt.Printf("  Services:   ")
		first := true
		for svc, count := range services {
			if !first {
				fmt.Printf(", ")
			}
			fmt.Printf("%s(%d)", svc, count)
			first = false
		}
		fmt.Println()
		fmt.Println()
	}

	// Display Network Status
	if networkStatus != nil {
		fmt.Println("🌍 NETWORK STATUS")
		fmt.Println("-" + strings.Repeat("-", 78))
		
		if networkStatus.Network != nil {
			fmt.Printf("  Network Name:       %s\n", networkStatus.Network.NetworkName)
			fmt.Printf("  Partitions:         %d\n", len(networkStatus.Network.Partitions))
			
			// List partitions
			for _, p := range networkStatus.Network.Partitions {
				fmt.Printf("    • %s (%s)\n", p.ID, p.Type)
			}
			
			fmt.Printf("  Validators:         %d total\n", len(networkStatus.Network.Validators))
		}
		
		if networkStatus.ExecutorVersion != 0 {
			fmt.Printf("  Executor Version:   %s\n", networkStatus.ExecutorVersion.String())
		}
		
		fmt.Printf("  Directory Height:   %d (⚠️ cached - live on validators)\n", networkStatus.DirectoryHeight)
		fmt.Printf("  Major Block Height: %d\n", networkStatus.MajorBlockHeight)
		
		if networkStatus.Oracle != nil && networkStatus.Oracle.Price > 0 {
			fmt.Printf("  Oracle Price:       %d credits/ACME\n", networkStatus.Oracle.Price)
		}
		fmt.Println()
	}

	// Display Metrics for each partition
	fmt.Println("📊 PARTITION METRICS")
	fmt.Println("-" + strings.Repeat("-", 78))
	
	if networkStatus != nil && networkStatus.Network != nil {
		for _, partition := range networkStatus.Network.Partitions {
			m.displayPartitionMetrics(ctx, partition.ID)
		}
	}
	
	// Display Network Globals
	if networkStatus != nil && networkStatus.Globals != nil {
		fmt.Println("⚙️  NETWORK PARAMETERS")
		fmt.Println("-" + strings.Repeat("-", 78))
		
		g := networkStatus.Globals
		fmt.Printf("  Operator Threshold:     %d/%d\n", 
			g.OperatorAcceptThreshold.Numerator,
			g.OperatorAcceptThreshold.Denominator)
		fmt.Printf("  Validator Threshold:    %d/%d\n",
			g.ValidatorAcceptThreshold.Numerator,
			g.ValidatorAcceptThreshold.Denominator)
		if g.MajorBlockSchedule != "" {
			fmt.Printf("  Major Block Schedule:   %s\n", g.MajorBlockSchedule)
		}
		if g.Limits != nil {
			fmt.Printf("  Limits:\n")
			fmt.Printf("    • Data Entry Parts:   %d\n", g.Limits.DataEntryParts)
			fmt.Printf("    • Account Authorities: %d\n", g.Limits.AccountAuthorities)
			fmt.Printf("    • Book Pages:         %d\n", g.Limits.BookPages)
			fmt.Printf("    • Page Entries:       %d\n", g.Limits.PageEntries)
			fmt.Printf("    • Identity Accounts:  %d\n", g.Limits.IdentityAccounts)
		}
		fmt.Println()
	}

	fmt.Println("=" + strings.Repeat("=", 78))
	if m.interval > 0 {
		fmt.Printf("Refreshing every %v. Press Ctrl+C to exit.\n", m.interval)
	}

	return nil
}

func (m *NetworkMonitor) displayPartitionMetrics(ctx context.Context, partition string) {
	metrics, err := m.client.GetMetrics(ctx, partition)
	if err != nil {
		fmt.Printf("  %s: Error getting metrics\n", partition)
		return
	}
	
	if metrics != nil {
		tps := metrics.TPS
		status := "🟢"
		if tps == 0 {
			status = "🟡"
		}
		fmt.Printf("  %s %s: %.2f TPS\n", status, partition, tps)
	}
}

// Helper to get consensus status for a partition
func (m *NetworkMonitor) getConsensusForPartition(ctx context.Context, partition string) (*v3.ConsensusStatus, error) {
	// Note: This would need the node ID for the partition
	// For now, we'll skip this as it requires node-specific information
	return nil, nil
}

// openBrowser tries to open the URL in the default browser
func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
		args = []string{url}
	}

	err := exec.Command(cmd, args...).Start()
	if err != nil {
		fmt.Printf("Could not open browser automatically. Please visit %s manually.\n", url)
	} else {
		fmt.Printf("Browser opened to %s\n", url)
	}
}