// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

// analyzeURLFormat analyzes a URL string and returns a description of its format
func analyzeURLFormat(urlStr string) string {
	// Check if it's a URL with scheme
	if !strings.Contains(urlStr, "://") {
		return "Invalid URL format: missing scheme"
	}

	// Split into scheme and host parts
	parts := strings.Split(urlStr, "://")
	scheme := parts[0]
	hostPart := parts[1]

	// Check for port
	hasPort := strings.Contains(hostPart, ":")
	var port string
	if hasPort {
		portParts := strings.Split(hostPart, ":")
		if len(portParts) > 1 {
			port = strings.Split(portParts[1], "/")[0] // Remove path if present
		}
	}

	// Check for path
	hasPath := strings.Contains(hostPart, "/")
	var path string
	if hasPath {
		pathParts := strings.Split(hostPart, "/")
		if len(pathParts) > 1 {
			path = "/" + strings.Join(pathParts[1:], "/")
		}
	}

	// Build description
	var desc strings.Builder
	desc.WriteString(fmt.Sprintf("Scheme: %s", scheme))
	
	if hasPort {
		desc.WriteString(fmt.Sprintf(", Port: %s", port))
	} else {
		desc.WriteString(", Port: None")
	}

	if hasPath {
		desc.WriteString(fmt.Sprintf(", Path: %s", path))
	} else {
		desc.WriteString(", Path: None")
	}

	return desc.String()
}

// analyzeMultiaddrFormat analyzes a multiaddr string and returns a description of its format
func analyzeMultiaddrFormat(addr string) string {
	// Check if it's a valid multiaddr format
	if !strings.HasPrefix(addr, "/") {
		return "Invalid multiaddr format: must begin with /"
	}

	// Split into components
	components := strings.Split(addr, "/")
	components = components[1:] // Remove empty first element

	// Analyze components
	var hasIP, hasDNS, hasP2P, hasTCP, hasUDP, hasQUIC bool
	var ipVersion, dnsVersion string

	for i := 0; i < len(components); i++ {
		switch components[i] {
		case "ip4":
			hasIP = true
			ipVersion = "IPv4"
			i++ // Skip the IP address value
		case "ip6":
			hasIP = true
			ipVersion = "IPv6"
			i++ // Skip the IP address value
		case "dns":
			hasDNS = true
			dnsVersion = "DNS"
			i++ // Skip the DNS value
		case "dns4":
			hasDNS = true
			dnsVersion = "DNS (IPv4)"
			i++ // Skip the DNS value
		case "dns6":
			hasDNS = true
			dnsVersion = "DNS (IPv6)"
			i++ // Skip the DNS value
		case "tcp":
			hasTCP = true
			i++ // Skip the port value
		case "udp":
			hasUDP = true
			i++ // Skip the port value
		case "quic":
			hasQUIC = true
		case "p2p":
			hasP2P = true
			i++ // Skip the peer ID value
		}
	}

	// Build description
	var desc strings.Builder

	// Host component
	if hasIP {
		desc.WriteString(fmt.Sprintf("Host: %s", ipVersion))
	} else if hasDNS {
		desc.WriteString(fmt.Sprintf("Host: %s", dnsVersion))
	} else {
		desc.WriteString("Host: None")
	}

	// Protocol
	desc.WriteString(", Protocol: ")
	if hasTCP {
		desc.WriteString("TCP")
	} else if hasUDP {
		if hasQUIC {
			desc.WriteString("QUIC over UDP")
		} else {
			desc.WriteString("UDP")
		}
	} else {
		desc.WriteString("None")
	}

	// Peer ID
	if hasP2P {
		desc.WriteString(", Has P2P ID: Yes")
	} else {
		desc.WriteString(", Has P2P ID: No")
	}

	return desc.String()
}

func TestSimplePeerDiscovery(t *testing.T) {
	// Create a log directory if it doesn't exist
	logDir := "logs"
	err := os.MkdirAll(logDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create log directory: %v", err)
	}

	// Create a log file with timestamp
	timestamp := time.Now().Format("20060102-150405")
	logFilePath := fmt.Sprintf("%s/peer_discovery_%s.log", logDir, timestamp)
	logFile, err := os.Create(logFilePath)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}
	defer logFile.Close()

	// Create a multi-writer that writes to both stdout and the file
	multiWriter := io.MultiWriter(os.Stdout, logFile)

	// Create a logger
	logger := log.New(multiWriter, "[PeerDiscovery] ", log.LstdFlags)
	
	// Log the start of the test
	logger.Printf("======= PEER DISCOVERY ANALYSIS STARTED =======")
	logger.Printf("Test started at: %s", time.Now().Format(time.RFC3339))

	// Create the discovery utility
	discovery := NewSimplePeerDiscovery(logger)

	// Log section header for multiaddr extraction tests
	logger.Printf("\n===== MULTIADDR HOST EXTRACTION ANALYSIS =====\n")
	logger.Printf("Testing extraction of host components from different multiaddr formats")
	logger.Printf("This simulates how the peer discovery process extracts host information from validator addresses")

	// Test cases for multiaddr extraction
	multiaddrs := []string{
		"/ip4/65.108.73.121/tcp/16593/p2p/defidevs.acme",
		"/ip4/65.108.4.175/tcp/16593/p2p/Sphereon.acme",
		"/dns4/validator.lunanova.acme/tcp/16593",
		"/ip4/135.181.114.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY",
		"/ip4/65.21.231.58/tcp/26656",
		"/ip4/65.108.201.154/udp/16593/quic",
		"/ip6/2a01:4f9:3a:2c26::2/tcp/16593",
		"/dns/validator.tfa.acme/tcp/16593",
		"/p2p/QmcEPkctU9P7ZWBxAE8CX8qDuDjQDQfR9JuJLt6VfbGZoX",
	}

	logger.Printf("Analyzing %d multiaddr formats", len(multiaddrs))
	fmt.Println("\n=== Testing Multiaddr Host Extraction ===")
	
	var successCount, failureCount int
	for i, addr := range multiaddrs {
		logger.Printf("\nTEST CASE %d: %s", i+1, addr)
		logger.Printf("Format analysis: %s", analyzeMultiaddrFormat(addr))
		
		host, err := discovery.ExtractHostFromMultiaddr(addr)
		if err != nil {
			failureCount++
			logger.Printf("❌ EXTRACTION FAILED: %v", err)
			fmt.Printf("❌ Failed to extract host from %s: %v\n", addr, err)
		} else {
			successCount++
			logger.Printf("✅ EXTRACTION SUCCEEDED: Host = %s", host)
			endpoint := discovery.GetPeerRPCEndpoint(host)
			logger.Printf("RPC Endpoint: %s", endpoint)
			fmt.Printf("✅ Successfully extracted host from %s: %s\n", addr, host)
			fmt.Printf("   RPC Endpoint: %s\n", endpoint)
		}
	}
	
	logger.Printf("\nMULTIADDR EXTRACTION SUMMARY:")
	logger.Printf("Total tested: %d", len(multiaddrs))
	logger.Printf("Successful extractions: %d (%.1f%%)", successCount, float64(successCount)/float64(len(multiaddrs))*100)
	logger.Printf("Failed extractions: %d (%.1f%%)", failureCount, float64(failureCount)/float64(len(multiaddrs))*100)

	// Log section header for URL extraction tests
	logger.Printf("\n===== URL HOST EXTRACTION ANALYSIS =====\n")
	logger.Printf("Testing extraction of host components from different URL formats")
	logger.Printf("This simulates how the peer discovery process extracts host information from validator addresses")

	// Test cases for URL extraction
	urls := []string{
		"http://65.108.73.121:16592",
		"https://validator.lunanova.acme:16592/ws",
		"ws://65.108.4.175:16593",
		"http://135.181.114.121",
		"https://validator.tfa.acme/rpc",
		"tcp://65.21.231.58:26656",
	}

	logger.Printf("Analyzing %d URL formats", len(urls))
	fmt.Println("\n=== Testing URL Host Extraction ===")
	
	var urlSuccessCount, urlFailureCount int
	for i, url := range urls {
		logger.Printf("\nURL TEST CASE %d: %s", i+1, url)
		logger.Printf("Format analysis: %s", analyzeURLFormat(url))
		
		host, err := discovery.ExtractHostFromURL(url)
		if err != nil {
			urlFailureCount++
			logger.Printf("❌ URL EXTRACTION FAILED: %v", err)
			fmt.Printf("❌ Failed to extract host from %s: %v\n", url, err)
		} else {
			urlSuccessCount++
			logger.Printf("✅ URL EXTRACTION SUCCEEDED: Host = %s", host)
			endpoint := discovery.GetPeerRPCEndpoint(host)
			logger.Printf("RPC Endpoint: %s", endpoint)
			fmt.Printf("✅ Successfully extracted host from %s: %s\n", url, host)
			fmt.Printf("   RPC Endpoint: %s\n", endpoint)
		}
	}
	
	logger.Printf("\nURL EXTRACTION SUMMARY:")
	logger.Printf("Total tested: %d", len(urls))
	logger.Printf("Successful extractions: %d (%.1f%%)", urlSuccessCount, float64(urlSuccessCount)/float64(len(urls))*100)
	logger.Printf("Failed extractions: %d (%.1f%%)", urlFailureCount, float64(urlFailureCount)/float64(len(urls))*100)

	// Log section header for validator address tests
	logger.Printf("\n===== VALIDATOR ADDRESS ANALYSIS =====\n")
	logger.Printf("Testing extraction of host information from validator IDs")
	logger.Printf("This simulates how the peer discovery process maps validator IDs to IP addresses")

	// Test known validator addresses
	validators := []string{
		"defidevs.acme",
		"LunaNova.acme",
		"Sphereon.acme",
		"ConsensusNetworks.acme",
		"tfa.acme",
		"HighStakes.acme",
		"TurtleBoat.acme",
		"UnknownValidator.acme",
	}

	logger.Printf("Analyzing %d validator IDs", len(validators))
	fmt.Println("\n=== Testing Known Validator Addresses ===")
	
	var validatorSuccessCount, validatorFailureCount int
	for i, validator := range validators {
		logger.Printf("\nVALIDATOR TEST CASE %d: %s", i+1, validator)
		
		// Try to get a known address for the validator
		host := discovery.GetKnownAddressForValidator(validator)
		if host == "" {
			validatorFailureCount++
			logger.Printf("❌ NO KNOWN ADDRESS: Could not find IP address for validator %s", validator)
			fmt.Printf("❌ No known address for validator %s\n", validator)
		} else {
			validatorSuccessCount++
			endpoint := discovery.GetPeerRPCEndpoint(host)
			logger.Printf("✅ FOUND ADDRESS: %s", host)
			logger.Printf("RPC Endpoint: %s", endpoint)
			fmt.Printf("✅ Found known address for validator %s: %s\n", validator, host)
			fmt.Printf("   RPC Endpoint: %s\n", endpoint)
		}
	}
	
	logger.Printf("\nVALIDATOR ADDRESS LOOKUP SUMMARY:")
	logger.Printf("Total tested: %d", len(validators))
	logger.Printf("Successful lookups: %d (%.1f%%)", validatorSuccessCount, float64(validatorSuccessCount)/float64(len(validators))*100)
	logger.Printf("Failed lookups: %d (%.1f%%)", validatorFailureCount, float64(validatorFailureCount)/float64(len(validators))*100)

	// Log section header for comparison tests
	logger.Printf("\n===== IMPLEMENTATION COMPARISON ANALYSIS =====\n")
	logger.Printf("Comparing our simple peer discovery implementation with the current AddressDir implementation")
	logger.Printf("This helps identify differences in how IP addresses are resolved from validator IDs")

	// Compare with our current implementation
	fmt.Println("\n=== Comparison with Current Implementation ===")
	addrDir := NewAddressDir()
	
	var matchCount, mismatchCount int
	logger.Printf("Comparing implementations for %d validators", len(validators))
	
	for i, validator := range validators {
		if host := discovery.GetKnownAddressForValidator(validator); host != "" {
			logger.Printf("\nCOMPARISON TEST CASE %d: %s", i+1, validator)
			
			// Create a NetworkPeer object for our current implementation
			networkPeer := NetworkPeer{
				ID:          "test-id",
				ValidatorID: validator,
				PartitionID: "Apollo", // Assuming Apollo partition for testing
				IsValidator: true,
			}
			
			// Get RPC endpoints from both implementations
			simplePeerEndpoint := discovery.GetPeerRPCEndpoint(host)
			currentEndpoint := addrDir.GetPeerRPCEndpoint(networkPeer)
			
			// Log the comparison results
			logger.Printf("Simple implementation: %s", simplePeerEndpoint)
			logger.Printf("Current implementation: %s", currentEndpoint)
			
			if simplePeerEndpoint == currentEndpoint {
				matchCount++
				logger.Printf("✅ MATCH: Both implementations return the same endpoint")
			} else {
				mismatchCount++
				logger.Printf("❌ MISMATCH: Implementations return different endpoints")
			}
			
			fmt.Printf("Validator: %s\n", validator)
			fmt.Printf("  Simple implementation: %s\n", simplePeerEndpoint)
			fmt.Printf("  Current implementation: %s\n", currentEndpoint)
			fmt.Printf("  Match: %v\n\n", simplePeerEndpoint == currentEndpoint)
		}
	}
	
	logger.Printf("\nIMPLEMENTATION COMPARISON SUMMARY:")
	logger.Printf("Total compared: %d", matchCount + mismatchCount)
	logger.Printf("Matching endpoints: %d (%.1f%%)", matchCount, float64(matchCount)/float64(matchCount+mismatchCount)*100)
	logger.Printf("Mismatched endpoints: %d (%.1f%%)", mismatchCount, float64(mismatchCount)/float64(matchCount+mismatchCount)*100)

	// Test edge cases
	fmt.Println("\n=== Testing Edge Cases ===")
	edgeCases := []string{
		"/p2p/QmcEPkctU9P7ZWBxAE8CX8qDuDjQDQfR9JuJLt6VfbGZoX", // No IP or DNS
		"/tcp/16593/p2p/defidevs.acme",                        // No IP or DNS
		"invalid-multiaddr",                                    // Invalid multiaddr
		"",                                                     // Empty string
	}

	for _, addr := range edgeCases {
		host, err := discovery.ExtractHostFromMultiaddr(addr)
		if err != nil {
			fmt.Printf("❌ (Expected) Failed to extract host from %s: %v\n", addr, err)
		} else {
			fmt.Printf("✅ Successfully extracted host from %s: %s\n", addr, host)
		}
	}
}
