package server

import (
	"encoding/json"
	"testing"
)

func TestBootstrapClient_QueryBootstrapPeers(t *testing.T) {
	client := NewBootstrapClient()

	// Test mainnet DN
	t.Run("MainnetDN", func(t *testing.T) {
		peers, err := client.QueryBootstrapPeers("mainnet", "dn")
		if err != nil {
			t.Logf("Warning: Failed to query mainnet DN peers: %v", err)
			t.Skip("Skipping test - cannot reach apollo-mainnet")
		}

		t.Logf("Found %d DN peers", len(peers))
		for i, peer := range peers {
			t.Logf("Peer %d: NodeID=%s, Moniker=%s, Network=%s", i, peer.NodeID, peer.Moniker, peer.Network)
			t.Logf("  Multiaddr: %s", peer.Multiaddr)
		}
	})

	// Test mainnet BVN
	t.Run("MainnetBVN", func(t *testing.T) {
		peers, err := client.QueryBootstrapPeers("mainnet", "bvn")
		if err != nil {
			t.Logf("Warning: Failed to query mainnet BVN peers: %v", err)
			t.Skip("Skipping test - cannot reach apollo-mainnet")
		}

		t.Logf("Found %d BVN peers", len(peers))
		for i, peer := range peers {
			t.Logf("Peer %d: NodeID=%s, Moniker=%s, Network=%s", i, peer.NodeID, peer.Moniker, peer.Network)
			t.Logf("  Multiaddr: %s", peer.Multiaddr)
		}
	})
}

func TestBootstrapClient_CompareWithHardcoded(t *testing.T) {
	client := NewBootstrapClient()

	// Test DN comparison
	t.Run("CompareDN", func(t *testing.T) {
		comparison, err := client.CompareWithHardcoded("mainnet", "dn")
		if err != nil {
			t.Fatalf("Failed to compare DN peers: %v", err)
		}

		// Pretty print the comparison
		jsonBytes, _ := json.MarshalIndent(comparison, "", "  ")
		t.Logf("DN Comparison:\n%s", string(jsonBytes))

		// Check fields exist
		if _, ok := comparison["source"]; !ok {
			t.Error("Missing 'source' field")
		}
		if _, ok := comparison["bootstrap_peers"]; !ok {
			t.Error("Missing 'bootstrap_peers' field")
		}
		if _, ok := comparison["hardcoded_peers"]; !ok {
			t.Error("Missing 'hardcoded_peers' field")
		}
		if _, ok := comparison["peers_match"]; !ok {
			t.Error("Missing 'peers_match' field")
		}

		// Log match status
		peersMatch := comparison["peers_match"].(bool)
		source := comparison["source"].(string)
		t.Logf("DN Peers Match: %v (Source: %s)", peersMatch, source)

		if !peersMatch {
			t.Logf("WARNING: DN peers don't match!")
			t.Logf("  Bootstrap count: %v", comparison["bootstrap_count"])
			t.Logf("  Hardcoded count: %v", comparison["hardcoded_count"])
			t.Logf("  Matching count: %v", comparison["matching_count"])

			if inBootstrapOnly, ok := comparison["in_bootstrap_only"].([]string); ok && len(inBootstrapOnly) > 0 {
				t.Logf("  In bootstrap only: %v", inBootstrapOnly)
			}
			if inHardcodedOnly, ok := comparison["in_hardcoded_only"].([]string); ok && len(inHardcodedOnly) > 0 {
				t.Logf("  In hardcoded only: %v", inHardcodedOnly)
			}
		}
	})

	// Test BVN comparison
	t.Run("CompareBVN", func(t *testing.T) {
		comparison, err := client.CompareWithHardcoded("mainnet", "bvn")
		if err != nil {
			t.Fatalf("Failed to compare BVN peers: %v", err)
		}

		// Pretty print the comparison
		jsonBytes, _ := json.MarshalIndent(comparison, "", "  ")
		t.Logf("BVN Comparison:\n%s", string(jsonBytes))

		// Log match status
		peersMatch := comparison["peers_match"].(bool)
		source := comparison["source"].(string)
		t.Logf("BVN Peers Match: %v (Source: %s)", peersMatch, source)

		if !peersMatch {
			t.Logf("WARNING: BVN peers don't match!")
			t.Logf("  Bootstrap count: %v", comparison["bootstrap_count"])
			t.Logf("  Hardcoded count: %v", comparison["hardcoded_count"])
			t.Logf("  Matching count: %v", comparison["matching_count"])

			if inBootstrapOnly, ok := comparison["in_bootstrap_only"].([]string); ok && len(inBootstrapOnly) > 0 {
				t.Logf("  In bootstrap only: %v", inBootstrapOnly)
			}
			if inHardcodedOnly, ok := comparison["in_hardcoded_only"].([]string); ok && len(inHardcodedOnly) > 0 {
				t.Logf("  In hardcoded only: %v", inHardcodedOnly)
			}
		}
	})
}

func TestBootstrapClient_GetBootstrapPeersWithFallback(t *testing.T) {
	client := NewBootstrapClient()

	// Test fallback behavior
	t.Run("MainnetDNWithFallback", func(t *testing.T) {
		peers, source, err := client.GetBootstrapPeersWithFallback("mainnet", "dn")
		if err != nil {
			t.Fatalf("Failed to get bootstrap peers with fallback: %v", err)
		}

		t.Logf("Source: %s", source)
		t.Logf("Peers (%d):", len(peers))
		for i, peer := range peers {
			t.Logf("  %d: %s", i, peer)
		}

		if len(peers) == 0 {
			t.Error("Expected at least one peer (hardcoded fallback should provide peers)")
		}
	})
}

func TestExtractNodeIDFromMultiaddr(t *testing.T) {
	tests := []struct {
		multiaddr  string
		expectedID string
	}{
		{
			multiaddr:  "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx",
			expectedID: "12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx",
		},
		{
			multiaddr:  "/ip4/23.22.212.106/tcp/16593/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD",
			expectedID: "QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD",
		},
		{
			multiaddr:  "/dns/invalid-without-p2p/tcp/16593",
			expectedID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.multiaddr, func(t *testing.T) {
			id := ExtractNodeIDFromMultiaddr(tt.multiaddr)
			if id != tt.expectedID {
				t.Errorf("ExtractNodeIDFromMultiaddr(%q) = %q, want %q", tt.multiaddr, id, tt.expectedID)
			}
		})
	}
}
