// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package load_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// FindDevnetEndpoint attempts to discover a running devnet endpoint
func FindDevnetEndpoint() (string, error) {
	// Check environment variable first
	if endpoint := os.Getenv("DEVNET_ENDPOINT"); endpoint != "" {
		return endpoint, nil
	}

	// Try to find accumulated process and its ports
	accumulatedPorts := findAccumulatedPorts()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First try ports from the actual process
	if len(accumulatedPorts) > 0 {
		for _, port := range accumulatedPorts {
			endpoint := fmt.Sprintf("http://localhost:%d/v3", port)
			if testEndpoint(ctx, endpoint) {
				return endpoint, nil
			}
		}
	}

	// Fallback to common devnet ports
	commonPorts := []int{
		26660, // BVN0
		26760, // BVN1
		26860, // BVN2
		26960, // DN
		8080,  // Default local port
		9090,  // Alternative port
	}

	for _, port := range commonPorts {
		// Skip if already checked
		alreadyChecked := false
		for _, p := range accumulatedPorts {
			if p == port {
				alreadyChecked = true
				break
			}
		}
		if alreadyChecked {
			continue
		}

		endpoint := fmt.Sprintf("http://localhost:%d/v3", port)
		if testEndpoint(ctx, endpoint) {
			return endpoint, nil
		}
	}

	// Check if accumulated process is running
	cmd := exec.Command("ps", "aux")
	output, _ := cmd.Output()
	if strings.Contains(string(output), "accumulated run devnet") {
		if len(accumulatedPorts) > 0 {
			return "", fmt.Errorf("accumulated devnet process is running on ports %v but API not accessible", accumulatedPorts)
		}
		return "", fmt.Errorf("accumulated devnet process is running but API endpoint not accessible on common ports")
	}

	return "", fmt.Errorf("no devnet is running - start with: ./accumulated run devnet")
}

// findAccumulatedPorts finds the ports that the accumulated process is listening on
func findAccumulatedPorts() []int {
	// First find the accumulated process PID
	cmd := exec.Command("pgrep", "-f", "accumulated run devnet")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	pid := strings.TrimSpace(string(output))
	if pid == "" {
		return nil
	}

	// Use lsof to find listening ports for this PID
	cmd = exec.Command("lsof", "-Pan", "-p", pid, "-i")
	output, err = cmd.Output()
	if err != nil {
		// Try alternative method with ss
		cmd = exec.Command("ss", "-tlnp")
		output, err = cmd.Output()
		if err != nil {
			return nil
		}
	}

	// Parse the output to find listening ports
	ports := make(map[int]bool)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Look for LISTEN state and extract port
		if strings.Contains(line, "LISTEN") {
			// Try to extract port from various formats
			// lsof format: *:26660 (LISTEN)
			// ss format: *:26660
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.Contains(part, ":") {
					portStr := strings.Split(part, ":")[len(strings.Split(part, ":"))-1]
					// Remove any trailing characters
					portStr = strings.TrimRight(portStr, " )")
					if port, err := strconv.Atoi(portStr); err == nil && port > 1024 && port < 65536 {
						ports[port] = true
					}
				}
			}
		}
	}

	// Convert map to slice
	var result []int
	for port := range ports {
		result = append(result, port)
	}

	// Sort ports for consistent ordering
	sort.Ints(result)
	return result
}

// testEndpoint tests if an endpoint is a valid Accumulate API endpoint
func testEndpoint(ctx context.Context, endpoint string) bool {
	client := jsonrpc.NewClient(endpoint)
	client.Client.Timeout = 1 * time.Second

	// Try to get network status
	_, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	return err == nil
}
