// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package load_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// DevnetEndpointFinder provides intelligent endpoint discovery for tests
type DevnetEndpointFinder struct {
	workDir       string
	discoveryFile string
	endpoints     []string
	apiClient     *jsonrpc.Client
}

// NewDevnetEndpointFinder creates a new endpoint finder
func NewDevnetEndpointFinder() *DevnetEndpointFinder {
	workDir := os.Getenv("DEVNET_WORK_DIR")
	if workDir == "" {
		workDir = ".devnet-test"
	}
	
	return &DevnetEndpointFinder{
		workDir:       workDir,
		discoveryFile: filepath.Join(workDir, "devnet-discovery.json"),
	}
}

// FindEndpoint discovers and returns a working devnet endpoint
func (f *DevnetEndpointFinder) FindEndpoint(t *testing.T) string {
	// 1. Check environment variable first
	if endpoint := os.Getenv("DEVNET_ENDPOINT"); endpoint != "" {
		if f.testEndpoint(endpoint) {
			t.Logf("Using DEVNET_ENDPOINT from environment: %s", endpoint)
			return endpoint
		}
		t.Logf("DEVNET_ENDPOINT %s not responding, searching for alternatives...", endpoint)
	}
	
	// 2. Try to load from discovery file
	if endpoint := f.loadFromDiscoveryFile(t); endpoint != "" {
		return endpoint
	}
	
	// 3. Scan for running processes and their ports
	if endpoint := f.scanRunningProcesses(t); endpoint != "" {
		return endpoint
	}
	
	// 4. Try common port ranges
	if endpoint := f.scanCommonPorts(t); endpoint != "" {
		return endpoint
	}
	
	// 5. Check if devnet needs to be started
	if !f.isDevnetRunning() {
		t.Log("No devnet found running. Please start devnet with one of:")
		t.Log("  ./devnet_config.sh start")
		t.Log("  go run ./cmd/accumulated run devnet")
		t.Log("  Or set DEVNET_ENDPOINT environment variable")
	}
	
	return ""
}

// loadFromDiscoveryFile attempts to load endpoints from the discovery file
func (f *DevnetEndpointFinder) loadFromDiscoveryFile(t *testing.T) string {
	data, err := os.ReadFile(f.discoveryFile)
	if err != nil {
		return ""
	}
	
	var discovery struct {
		Endpoints map[string]string `json:"endpoints"`
		Nodes     map[string]struct {
			IP    string `json:"ip"`
			Ports struct {
				API int `json:"api"`
			} `json:"ports"`
		} `json:"nodes"`
		Updated time.Time `json:"updated"`
	}
	
	if err := json.Unmarshal(data, &discovery); err != nil {
		return ""
	}
	
	// Check if discovery info is recent (less than 5 minutes old)
	if time.Since(discovery.Updated) > 5*time.Minute {
		t.Logf("Discovery file is stale (updated %v ago)", time.Since(discovery.Updated))
		return ""
	}
	
	// Try endpoints from discovery file
	for name, endpoint := range discovery.Endpoints {
		if strings.Contains(name, "api") || name == "bootstrap" {
			if f.testEndpoint(endpoint) {
				t.Logf("Found working endpoint from discovery file: %s (%s)", endpoint, name)
				return endpoint
			}
		}
	}
	
	// Try constructing endpoints from node info
	for name, node := range discovery.Nodes {
		if node.Ports.API > 0 {
			endpoint := fmt.Sprintf("http://%s:%d/v3", node.IP, node.Ports.API)
			if f.testEndpoint(endpoint) {
				t.Logf("Found working endpoint from node %s: %s", name, endpoint)
				return endpoint
			}
		}
	}
	
	return ""
}

// scanRunningProcesses finds accumulated processes and their listening ports
func (f *DevnetEndpointFinder) scanRunningProcesses(t *testing.T) string {
	// Find accumulated process PIDs
	pids := f.findAccumulatedPIDs()
	if len(pids) == 0 {
		return ""
	}
	
	t.Logf("Found %d accumulated process(es)", len(pids))
	
	// Get listening ports for these PIDs
	ports := f.getListeningPorts(pids)
	if len(ports) == 0 {
		return ""
	}
	
	t.Logf("Found %d listening port(s): %v", len(ports), ports)
	
	// Test each port as a potential API endpoint
	for _, port := range ports {
		// Try both 127.0.0.1 and the discovered IP
		for _, ip := range f.getPossibleIPs(port) {
			endpoint := fmt.Sprintf("http://%s:%d/v3", ip, port)
			if f.testEndpoint(endpoint) {
				t.Logf("Found working endpoint: %s", endpoint)
				return endpoint
			}
		}
	}
	
	return ""
}

// findAccumulatedPIDs finds PIDs of accumulated processes
func (f *DevnetEndpointFinder) findAccumulatedPIDs() []string {
	cmd := exec.Command("pgrep", "-f", "accumulated.*devnet")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var pids []string
	for _, line := range lines {
		if line != "" {
			pids = append(pids, line)
		}
	}
	return pids
}

// getListeningPorts gets listening ports for given PIDs
func (f *DevnetEndpointFinder) getListeningPorts(pids []string) []int {
	ports := make(map[int]bool)
	
	for _, pid := range pids {
		// Try lsof first
		cmd := exec.Command("lsof", "-Pan", "-p", pid, "-iTCP", "-sTCP:LISTEN")
		output, _ := cmd.Output()
		
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "LISTEN") {
				// Extract port from line like "TCP *:26660 (LISTEN)"
				parts := strings.Fields(line)
				for _, part := range parts {
					if strings.Contains(part, ":") {
						portStr := part[strings.LastIndex(part, ":")+1:]
						var port int
						if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil {
							ports[port] = true
						}
					}
				}
			}
		}
	}
	
	// Also try ss command as fallback
	if len(ports) == 0 {
		cmd := exec.Command("ss", "-tlnp")
		output, _ := cmd.Output()
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "accumulated") {
				// Extract port
				fields := strings.Fields(line)
				for _, field := range fields {
					if strings.Contains(field, ":") {
						parts := strings.Split(field, ":")
						if len(parts) >= 2 {
							var port int
							if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &port); err == nil {
								ports[port] = true
							}
						}
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
	return result
}

// getPossibleIPs returns possible IP addresses to try
func (f *DevnetEndpointFinder) getPossibleIPs(port int) []string {
	ips := []string{"127.0.0.1", "localhost"}
	
	// Try to detect the actual IP from ss output
	cmd := exec.Command("ss", "-tln")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, fmt.Sprintf(":%d", port)) {
				// Extract IP address
				fields := strings.Fields(line)
				for _, field := range fields {
					if strings.Contains(field, fmt.Sprintf(":%d", port)) {
						parts := strings.Split(field, ":")
						if len(parts) >= 2 {
							ip := strings.Join(parts[:len(parts)-1], ":")
							ip = strings.TrimPrefix(ip, "[")
							ip = strings.TrimSuffix(ip, "]")
							if ip != "*" && ip != "0.0.0.0" && ip != "" {
								// Add this IP if it's not already in the list
								found := false
								for _, existing := range ips {
									if existing == ip {
										found = true
										break
									}
								}
								if !found {
									ips = append([]string{ip}, ips...) // Prepend to try first
								}
							}
						}
					}
				}
			}
		}
	}
	
	return ips
}

// scanCommonPorts scans common devnet ports
func (f *DevnetEndpointFinder) scanCommonPorts(t *testing.T) string {
	t.Log("Scanning common devnet ports...")
	
	// Common port patterns for devnet
	commonPorts := []int{
		26660, 26760, 26860, 26960, // BVN API ports
		26661, 26761, 26861, 26961, // Alternative API ports
		8080, 8081, 8082,           // Common alternative ports
		9090, 9091, 9092,           // More alternatives
	}
	
	// Try different IP ranges
	ipRanges := []string{
		"127.0.0.%d",   // 127.0.0.x
		"127.0.1.%d",   // 127.0.1.x (devnet default)
		"localhost",    // localhost
	}
	
	for _, port := range commonPorts {
		for _, ipPattern := range ipRanges {
			var ips []string
			if ipPattern == "localhost" {
				ips = []string{"localhost"}
			} else {
				// Try first 20 IPs in the range
				for i := 1; i <= 20; i++ {
					ips = append(ips, fmt.Sprintf(ipPattern, i))
				}
			}
			
			for _, ip := range ips {
				endpoint := fmt.Sprintf("http://%s:%d/v3", ip, port)
				if f.testEndpointQuick(endpoint) {
					t.Logf("Found working endpoint: %s", endpoint)
					return endpoint
				}
			}
		}
	}
	
	return ""
}

// testEndpoint tests if an endpoint is working
func (f *DevnetEndpointFinder) testEndpoint(endpoint string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	client := jsonrpc.NewClient(endpoint)
	client.Client.Timeout = 2 * time.Second
	
	// Try to get network status
	_, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	return err == nil
}

// testEndpointQuick does a quick TCP connection test
func (f *DevnetEndpointFinder) testEndpointQuick(endpoint string) bool {
	// Extract host:port from URL
	if strings.HasPrefix(endpoint, "http://") {
		endpoint = strings.TrimPrefix(endpoint, "http://")
	}
	if strings.HasSuffix(endpoint, "/v3") {
		endpoint = strings.TrimSuffix(endpoint, "/v3")
	}
	
	// Quick TCP connection test
	conn, err := net.DialTimeout("tcp", endpoint, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	
	// Now do a proper API test
	fullEndpoint := fmt.Sprintf("http://%s/v3", endpoint)
	return f.testEndpoint(fullEndpoint)
}

// isDevnetRunning checks if any devnet process is running
func (f *DevnetEndpointFinder) isDevnetRunning() bool {
	cmd := exec.Command("pgrep", "-f", "accumulated.*devnet")
	err := cmd.Run()
	return err == nil
}

// SaveDiscoveryInfo saves discovery information for other tests
func (f *DevnetEndpointFinder) SaveDiscoveryInfo(endpoint string) error {
	discovery := map[string]interface{}{
		"primary_endpoint": endpoint,
		"discovered_at":    time.Now(),
		"pid":             os.Getpid(),
	}
	
	// Ensure directory exists
	os.MkdirAll(filepath.Dir(f.discoveryFile), 0755)
	
	data, err := json.MarshalIndent(discovery, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(f.discoveryFile, data, 0644)
}

// GetOrStartDevnet attempts to find or start a devnet
func GetOrStartDevnet(t *testing.T) string {
	finder := NewDevnetEndpointFinder()
	
	// First try to find existing devnet
	endpoint := finder.FindEndpoint(t)
	if endpoint != "" {
		return endpoint
	}
	
	// No devnet found, try to start one
	t.Log("No devnet found, attempting to start one...")
	
	// Check if devnet_config.sh exists
	configScript := "./devnet_config.sh"
	if _, err := os.Stat(configScript); err == nil {
		t.Log("Starting devnet using devnet_config.sh...")
		cmd := exec.Command(configScript, "quick")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("Failed to start devnet: %v", err)
		}
		
		// Wait for devnet to be ready
		t.Log("Waiting for devnet to be ready...")
		time.Sleep(10 * time.Second)
		
		// Try to find endpoint again
		endpoint = finder.FindEndpoint(t)
		if endpoint != "" {
			finder.SaveDiscoveryInfo(endpoint)
			return endpoint
		}
	}
	
	t.Fatal("Failed to find or start devnet")
	return ""
}

// DiscoverPartitions discovers all available partitions in the network
func DiscoverPartitions(endpoint string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	client := jsonrpc.NewClient(endpoint)
	status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return nil, err
	}
	
	var partitions []string
	if status.Network != nil && status.Network.Partitions != nil {
		for _, p := range status.Network.Partitions {
			partitions = append(partitions, p.ID)
		}
	}
	
	return partitions, nil
}

// FindHealthyValidator finds a healthy validator endpoint for a specific partition
func FindHealthyValidator(partition string, baseEndpoint string) (string, error) {
	// This would query the network to find validators for the partition
	// For now, we'll use a simple heuristic based on port patterns
	
	// Extract base URL
	if idx := strings.Index(baseEndpoint, "://"); idx > 0 {
		baseEndpoint = baseEndpoint[idx+3:]
	}
	if idx := strings.Index(baseEndpoint, "/"); idx > 0 {
		baseEndpoint = baseEndpoint[:idx]
	}
	
	// Try common port offsets for different partitions
	portOffsets := map[string]int{
		"Directory": 0,
		"BVN0":      100,
		"BVN1":      200,
		"BVN2":      300,
	}
	
	if offset, ok := portOffsets[partition]; ok {
		parts := strings.Split(baseEndpoint, ":")
		if len(parts) == 2 {
			var basePort int
			fmt.Sscanf(parts[1], "%d", &basePort)
			newPort := basePort + offset
			endpoint := fmt.Sprintf("http://%s:%d/v3", parts[0], newPort)
			
			// Test if it works
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			
			client := jsonrpc.NewClient(endpoint)
			if _, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{}); err == nil {
				return endpoint, nil
			}
		}
	}
	
	return baseEndpoint, nil
}

// MonitorEndpointHealth continuously monitors endpoint health
func MonitorEndpointHealth(endpoint string, interval time.Duration) <-chan bool {
	health := make(chan bool, 1)
	
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			client := jsonrpc.NewClient(endpoint)
			_, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
			cancel()
			
			select {
			case health <- (err == nil):
			default:
				// Don't block if no one is reading
			}
		}
	}()
	
	return health
}