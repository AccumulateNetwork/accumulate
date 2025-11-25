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
	// Validate network name to prevent injection
	if err := ValidateNetworkName(network); err != nil {
		return nil, fmt.Errorf("invalid network: %w", err)
	}

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
		return nil, fmt.Errorf("unsupported network: %s (must be 'mainnet' or 'testnet')", network)
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

		// Validate the generated multiaddr before adding
		if err := ValidateMultiaddr(multiaddr); err != nil {
			// Skip invalid peers but continue processing
			continue
		}

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
		result := validateAndFilterMultiaddrs(hardcoded)
		return result, "hardcoded (bootstrap server unreachable: " + err.Error() + ")", nil
	}

	// Extract multiaddrs from peers (already validated in QueryBootstrapPeers)
	var multiaddrs []string
	for _, peer := range peers {
		multiaddrs = append(multiaddrs, peer.Multiaddr)
	}

	if len(multiaddrs) == 0 {
		// No peers found, fall back to hardcoded
		hardcoded := getDefaultBootstrapPeers(network, partition)
		result := validateAndFilterMultiaddrs(hardcoded)
		return result, "hardcoded (no peers found on bootstrap server)", nil
	}

	return multiaddrs, "queried from bootstrap server", nil
}

// validateAndFilterMultiaddrs validates and filters multiaddr strings from interface slice
func validateAndFilterMultiaddrs(addrs []interface{}) []string {
	var result []string
	for _, p := range addrs {
		if s, ok := p.(string); ok {
			if err := ValidateMultiaddr(s); err == nil {
				result = append(result, s)
			}
		}
	}
	return result
}

// CompareWithHardcoded compares bootstrap server results with hardcoded values
func (c *BootstrapClient) CompareWithHardcoded(network string, partition string) (map[string]interface{}, error) {
	// Get bootstrap server peers (already validated)
	bootstrapPeers, source, err := c.GetBootstrapPeersWithFallback(network, partition)
	if err != nil {
		return nil, err
	}

	// Get hardcoded peers (validate them)
	hardcodedRaw := getDefaultBootstrapPeers(network, partition)
	hardcodedPeers := validateAndFilterMultiaddrs(hardcodedRaw)

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

// BootstrapServerInfo represents the response from /info endpoint
type BootstrapServerInfo struct {
	PeerID            string         `json:"peer_id"`
	ListenAddresses   []string       `json:"listen_addresses"`
	ExternalAddresses []string       `json:"external_addresses"`
	DHT               DHTInfo        `json:"dht"`
	Connections       ConnectionInfo `json:"connections"`
	UptimeSeconds     int64          `json:"uptime_seconds"`
}

// DHTInfo contains DHT statistics
type DHTInfo struct {
	Mode             string `json:"mode"`
	RoutingTableSize int    `json:"routing_table_size"`
}

// ConnectionInfo contains connection statistics
type ConnectionInfo struct {
	Total    int `json:"total"`
	Inbound  int `json:"inbound"`
	Outbound int `json:"outbound"`
}

// BootstrapServerHealth represents the response from /health endpoint
type BootstrapServerHealth struct {
	Status      string `json:"status"`
	PeerCount   int    `json:"peer_count"`
	ConnCount   int    `json:"conn_count"`
	UptimeHours int64  `json:"uptime_hours"`
	Reason      string `json:"reason,omitempty"`
}

// BootstrapPeer represents a single peer from /peers endpoint
type BootstrapPeer struct {
	PeerID    string   `json:"peer_id"`
	Addresses []string `json:"addresses"`
}

// BootstrapPeersResponse represents the response from /peers endpoint
type BootstrapPeersResponse struct {
	Count int             `json:"count"`
	Peers []BootstrapPeer `json:"peers"`
}

// QueryBootstrapServerInfo queries the bootstrap server's /info endpoint
func (c *BootstrapClient) QueryBootstrapServerInfo(serverURL string) (*BootstrapServerInfo, error) {
	if serverURL == "" {
		serverURL = "http://bootstrap.accumulate.defidevs.io:8080"
	}

	url := fmt.Sprintf("%s/info", serverURL)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to query bootstrap server at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bootstrap server returned status %d: %s", resp.StatusCode, string(body))
	}

	var info BootstrapServerInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &info, nil
}

// QueryBootstrapServerHealth queries the bootstrap server's /health endpoint
func (c *BootstrapClient) QueryBootstrapServerHealth(serverURL string) (*BootstrapServerHealth, error) {
	if serverURL == "" {
		serverURL = "http://bootstrap.accumulate.defidevs.io:8080"
	}

	url := fmt.Sprintf("%s/health", serverURL)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to query bootstrap server at %s: %w", url, err)
	}
	defer resp.Body.Close()

	var health BootstrapServerHealth
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &health, nil
}

// QueryBootstrapServerPeers queries the bootstrap server's /peers endpoint
func (c *BootstrapClient) QueryBootstrapServerPeers(serverURL string) (*BootstrapPeersResponse, error) {
	if serverURL == "" {
		serverURL = "http://bootstrap.accumulate.defidevs.io:8080"
	}

	url := fmt.Sprintf("%s/peers", serverURL)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to query bootstrap server at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bootstrap server returned status %d: %s", resp.StatusCode, string(body))
	}

	var peers BootstrapPeersResponse
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &peers, nil
}
