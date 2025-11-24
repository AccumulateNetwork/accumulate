package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// BootstrapClient queries the bootstrap server for network information
type BootstrapClient struct {
	httpClient *http.Client
}

// NewBootstrapClient creates a new bootstrap client
func NewBootstrapClient() *BootstrapClient {
	return &BootstrapClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// TendermintNetInfo represents the response from /net_info endpoint
type TendermintNetInfo struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  struct {
		Listening bool `json:"listening"`
		Peers     []struct {
			NodeInfo struct {
				ProtocolVersion struct {
					P2P   string `json:"p2p"`
					Block string `json:"block"`
					App   string `json:"app"`
				} `json:"protocol_version"`
				ID         string `json:"id"`
				ListenAddr string `json:"listen_addr"`
				Network    string `json:"network"`
				Version    string `json:"version"`
				Channels   string `json:"channels"`
				Moniker    string `json:"moniker"`
				Other      struct {
					TxIndex    string `json:"tx_index"`
					RPCAddress string `json:"rpc_address"`
				} `json:"other"`
			} `json:"node_info"`
			IsOutbound       bool `json:"is_outbound"`
			ConnectionStatus struct {
				Duration    string `json:"Duration"`
				SendMonitor struct {
					Active   bool   `json:"Active"`
					Start    string `json:"Start"`
					Duration string `json:"Duration"`
					Idle     string `json:"Idle"`
					Bytes    string `json:"Bytes"`
					Samples  string `json:"Samples"`
					InstRate string `json:"InstRate"`
					CurRate  string `json:"CurRate"`
					AvgRate  string `json:"AvgRate"`
					PeakRate string `json:"PeakRate"`
				} `json:"SendMonitor"`
				RecvMonitor struct {
					Active   bool   `json:"Active"`
					Start    string `json:"Start"`
					Duration string `json:"Duration"`
					Idle     string `json:"Idle"`
					Bytes    string `json:"Bytes"`
					Samples  string `json:"Samples"`
					InstRate string `json:"InstRate"`
					CurRate  string `json:"CurRate"`
					AvgRate  string `json:"AvgRate"`
					PeakRate string `json:"PeakRate"`
				} `json:"RecvMonitor"`
				Channels []struct {
					ID                byte   `json:"ID"`
					SendQueueCapacity string `json:"SendQueueCapacity"`
					SendQueueSize     string `json:"SendQueueSize"`
					Priority          string `json:"Priority"`
					RecentlySent      string `json:"RecentlySent"`
				} `json:"Channels"`
			} `json:"connection_status"`
			RemoteIP string `json:"remote_ip"`
		} `json:"peers"`
	} `json:"result"`
}

// BootstrapPeerInfo represents peer information from the bootstrap server
type BootstrapPeerInfo struct {
	NodeID     string `json:"node_id"`
	ListenAddr string `json:"listen_addr"`
	Moniker    string `json:"moniker"`
	Network    string `json:"network"`
	Multiaddr  string `json:"multiaddr"`
}

// QueryBootstrapPeers queries the validator nodes for peer information
// Note: The "bootstrap server" (bootstrap.accumulate.defidevs.io) is a libp2p DHT server,
// not a CometBFT node. To get network peer info, we query apollo-mainnet (the validator).
func (c *BootstrapClient) QueryBootstrapPeers(network string, partition string) ([]BootstrapPeerInfo, error) {
	// Determine validator endpoint based on network and partition
	// We query the actual validator (apollo-mainnet) not the bootstrap server
	var baseURL string
	var tmPort int

	switch network {
	case "mainnet":
		baseURL = "apollo-mainnet.accumulate.defidevs.io"
		if partition == "dn" {
			tmPort = 16592 // DN CometBFT RPC port
		} else {
			tmPort = 16692 // BVN CometBFT RPC port
		}
	case "testnet":
		baseURL = "testnet.accumulate.defidevs.io"
		if partition == "dn" {
			tmPort = 16592
		} else {
			tmPort = 16692
		}
	default:
		return nil, fmt.Errorf("unsupported network: %s", network)
	}

	// Query the /net_info endpoint
	url := fmt.Sprintf("http://%s:%d/net_info", baseURL, tmPort)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to query bootstrap server at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bootstrap server returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var netInfo TendermintNetInfo
	if err := json.Unmarshal(body, &netInfo); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert peers to BootstrapPeerInfo
	var peers []BootstrapPeerInfo
	for _, peer := range netInfo.Result.Peers {
		// Extract host and port from ListenAddr
		// Format is typically: tcp://0.0.0.0:16593 or similar
		listenAddr := peer.NodeInfo.ListenAddr

		// Convert to multiaddr format
		// We need to use the bootstrap server's hostname, not the peer's listen address
		// because listen addresses are often 0.0.0.0 or internal IPs
		var p2pPort int
		if partition == "dn" {
			p2pPort = 16593 // DN P2P port
		} else {
			p2pPort = 16693 // BVN P2P port
		}

		multiaddr := fmt.Sprintf("/dns/%s/tcp/%d/p2p/%s", baseURL, p2pPort, peer.NodeInfo.ID)

		peers = append(peers, BootstrapPeerInfo{
			NodeID:     peer.NodeInfo.ID,
			ListenAddr: listenAddr,
			Moniker:    peer.NodeInfo.Moniker,
			Network:    peer.NodeInfo.Network,
			Multiaddr:  multiaddr,
		})
	}

	return peers, nil
}

// GetBootstrapPeersWithFallback queries the bootstrap server and falls back to hardcoded values
func (c *BootstrapClient) GetBootstrapPeersWithFallback(network string, partition string) ([]string, string, error) {
	// Try to query the bootstrap server
	peers, err := c.QueryBootstrapPeers(network, partition)
	if err != nil {
		// Fall back to hardcoded values
		hardcoded := getDefaultBootstrapPeers(network, partition)
		var result []string
		for _, p := range hardcoded {
			if s, ok := p.(string); ok {
				result = append(result, s)
			}
		}
		return result, "hardcoded (bootstrap server unreachable: " + err.Error() + ")", nil
	}

	// Extract multiaddrs from peers
	var multiaddrs []string
	for _, peer := range peers {
		multiaddrs = append(multiaddrs, peer.Multiaddr)
	}

	if len(multiaddrs) == 0 {
		// No peers found, fall back to hardcoded
		hardcoded := getDefaultBootstrapPeers(network, partition)
		var result []string
		for _, p := range hardcoded {
			if s, ok := p.(string); ok {
				result = append(result, s)
			}
		}
		return result, "hardcoded (no peers found on bootstrap server)", nil
	}

	return multiaddrs, "queried from bootstrap server", nil
}

// CompareWithHardcoded compares bootstrap server results with hardcoded values
func (c *BootstrapClient) CompareWithHardcoded(network string, partition string) (map[string]interface{}, error) {
	// Get bootstrap server peers
	bootstrapPeers, source, err := c.GetBootstrapPeersWithFallback(network, partition)
	if err != nil {
		return nil, err
	}

	// Get hardcoded peers
	hardcodedRaw := getDefaultBootstrapPeers(network, partition)
	var hardcodedPeers []string
	for _, p := range hardcodedRaw {
		if s, ok := p.(string); ok {
			hardcodedPeers = append(hardcodedPeers, s)
		}
	}

	// Compare
	matching := []string{}
	inBootstrapOnly := []string{}
	inHardcodedOnly := []string{}

	// Build maps for easier comparison
	hardcodedMap := make(map[string]bool)
	for _, p := range hardcodedPeers {
		hardcodedMap[p] = true
	}

	bootstrapMap := make(map[string]bool)
	for _, p := range bootstrapPeers {
		bootstrapMap[p] = true
	}

	// Find matching and bootstrap-only
	for _, p := range bootstrapPeers {
		if hardcodedMap[p] {
			matching = append(matching, p)
		} else {
			inBootstrapOnly = append(inBootstrapOnly, p)
		}
	}

	// Find hardcoded-only
	for _, p := range hardcodedPeers {
		if !bootstrapMap[p] {
			inHardcodedOnly = append(inHardcodedOnly, p)
		}
	}

	return map[string]interface{}{
		"network":              network,
		"partition":            partition,
		"source":               source,
		"bootstrap_peers":      bootstrapPeers,
		"hardcoded_peers":      hardcodedPeers,
		"matching_peers":       matching,
		"in_bootstrap_only":    inBootstrapOnly,
		"in_hardcoded_only":    inHardcodedOnly,
		"peers_match":          len(matching) == len(bootstrapPeers) && len(matching) == len(hardcodedPeers),
		"bootstrap_count":      len(bootstrapPeers),
		"hardcoded_count":      len(hardcodedPeers),
		"matching_count":       len(matching),
		"bootstrap_only_count": len(inBootstrapOnly),
		"hardcoded_only_count": len(inHardcodedOnly),
	}, nil
}

// ExtractNodeIDFromMultiaddr extracts the node ID from a multiaddr string
func ExtractNodeIDFromMultiaddr(multiaddr string) string {
	parts := strings.Split(multiaddr, "/p2p/")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}
