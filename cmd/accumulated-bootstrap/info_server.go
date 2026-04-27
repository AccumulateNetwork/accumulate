// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// BootstrapInfo contains information about the bootstrap server
type BootstrapInfo struct {
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

// HealthStatus contains health check status
type HealthStatus struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// PeerInfo contains information about a connected peer
type PeerInfo struct {
	PeerID    string   `json:"peer_id"`
	Addresses []string `json:"addresses"`
}

// PeersResponse contains the list of connected peers
type PeersResponse struct {
	Count int        `json:"count"`
	Peers []PeerInfo `json:"peers"`
}

// ConnectRequest contains a peer address to connect to
type ConnectRequest struct {
	Address string `json:"address"`
}

// ConnectResponse contains the result of a connection attempt
type ConnectResponse struct {
	Success bool   `json:"success"`
	PeerID  string `json:"peer_id,omitempty"`
	Error   string `json:"error,omitempty"`
}

// InfoServer serves bootstrap server information on HTTP
type InfoServer struct {
	host        host.Host
	server      *http.Server
	startTime   time.Time
	external    []multiaddr.Multiaddr
	metrics     *MetricsCollector
	partitions  *PartitionTracker
	connections *ConnectionManager
}

// NewInfoServer creates a new info server
func NewInfoServer(h host.Host, listen multiaddr.Multiaddr, external []multiaddr.Multiaddr) (*InfoServer, error) {
	metrics := NewMetricsCollector(h)
	s := &InfoServer{
		host:       h,
		startTime:  time.Now(),
		external:   external,
		metrics:    metrics,
		partitions: NewPartitionTracker(h, metrics),
	}

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/info", s.handleInfo)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/peers", s.handlePeers)
	mux.HandleFunc("/peers/", s.handlePeersByPartition)
	mux.HandleFunc("/partitions", s.handlePartitions)
	mux.HandleFunc("/connect", s.handleConnect)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/connections", s.handleConnections)
	mux.HandleFunc("/debug/dht", s.handleDebugDHT)

	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Start listening
	listener, err := listenMultiaddr(listen)
	if err != nil {
		return nil, err
	}

	go func() {
		slog.Info("Info server listening", "address", listener.Addr())
		err := s.server.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			slog.Error("Info server stopped", "error", err)
		}
	}()

	return s, nil
}

// handleInfo serves bootstrap server information
func (s *InfoServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Gather connection statistics
	conns := s.host.Network().Conns()
	inbound := 0
	outbound := 0
	for _, conn := range conns {
		if conn.Stat().Direction == network.DirInbound {
			inbound++
		} else {
			outbound++
		}
	}

	// Get peer addresses
	peers := s.host.Network().Peers()

	// Build response
	info := BootstrapInfo{
		PeerID:            s.host.ID().String(),
		ListenAddresses:   filterLocalAddresses(s.host.Addrs()),
		ExternalAddresses: buildExternalAddresses(s.host, s.external),
		DHT: DHTInfo{
			Mode:             "server",
			RoutingTableSize: len(peers),
		},
		Connections: ConnectionInfo{
			Total:    len(conns),
			Inbound:  inbound,
			Outbound: outbound,
		},
		UptimeSeconds: int64(time.Since(s.startTime).Seconds()),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(info); err != nil {
		slog.Error("Failed to encode info response", "error", err)
	}
}

// handleHealth serves health check
func (s *InfoServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	defer func() {
		status := http.StatusOK
		s.metrics.RecordHTTPRequest("/health", status, time.Since(start))
	}()

	// Check if we have any connections or peers
	peers := s.host.Network().Peers()
	conns := s.host.Network().Conns()

	// Gather connection statistics
	inbound := 0
	outbound := 0
	for _, conn := range conns {
		if conn.Stat().Direction == network.DirInbound {
			inbound++
		} else {
			outbound++
		}
	}

	// Determine health status and any issues
	issues := make([]string, 0)
	status := "healthy"
	httpStatus := http.StatusOK

	uptime := time.Since(s.startTime)

	// Check for no peers after startup grace period
	if len(peers) == 0 && uptime > 5*time.Minute {
		status = "degraded"
		issues = append(issues, "no peers in DHT routing table after 5 minutes")
	}

	// Check for no connections after startup
	if len(conns) == 0 && uptime > 2*time.Minute {
		status = "degraded"
		issues = append(issues, "no active connections after 2 minutes")
	}

	// Check for imbalanced connections (all inbound or all outbound)
	if len(conns) > 5 && (inbound == 0 || outbound == 0) {
		issues = append(issues, "connection direction imbalance")
	}

	// Set HTTP status based on health
	if status == "unhealthy" {
		httpStatus = http.StatusServiceUnavailable
	} else if status == "degraded" {
		httpStatus = http.StatusOK // Still return 200 for degraded to avoid alerting
	}

	// Build detailed health response
	healthDetails := map[string]interface{}{
		"status": status,
		"checks": map[string]interface{}{
			"peers": map[string]interface{}{
				"count":  len(peers),
				"status": boolToStatus(len(peers) > 0 || uptime < 5*time.Minute),
			},
			"connections": map[string]interface{}{
				"total":    len(conns),
				"inbound":  inbound,
				"outbound": outbound,
				"status":   boolToStatus(len(conns) > 0 || uptime < 2*time.Minute),
			},
			"dht": map[string]interface{}{
				"routing_table_size": len(peers),
				"status":             boolToStatus(len(peers) > 0 || uptime < 5*time.Minute),
			},
		},
		"uptime": map[string]interface{}{
			"seconds": int64(uptime.Seconds()),
			"hours":   uptime.Hours(),
			"human":   formatDuration(uptime),
		},
		"server_start_time": s.startTime.Format(time.RFC3339),
	}

	if len(issues) > 0 {
		healthDetails["issues"] = issues
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(healthDetails); err != nil {
		slog.Error("Failed to encode health response", "error", err)
	}
}

// boolToStatus converts a boolean to "pass" or "warn" status
func boolToStatus(ok bool) string {
	if ok {
		return "pass"
	}
	return "warn"
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// handlePeers serves the list of connected peers
func (s *InfoServer) handlePeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get list of connected peers
	peers := s.host.Network().Peers()

	// Build peer info list
	peerInfos := make([]PeerInfo, 0, len(peers))
	for _, peerID := range peers {
		// Get addresses for this peer
		addrs := s.host.Peerstore().Addrs(peerID)
		addrStrs := make([]string, 0, len(addrs))
		for _, addr := range addrs {
			// Build full multiaddr with peer ID
			fullAddr := addr.String() + "/p2p/" + peerID.String()
			addrStrs = append(addrStrs, fullAddr)
		}

		peerInfos = append(peerInfos, PeerInfo{
			PeerID:    peerID.String(),
			Addresses: addrStrs,
		})
	}

	response := PeersResponse{
		Count: len(peerInfos),
		Peers: peerInfos,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode peers response", "error", err)
	}
}

// handleConnect attempts to connect to a peer
func (s *InfoServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req ConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ConnectResponse{
			Success: false,
			Error:   "invalid request body: " + err.Error(),
		})
		return
	}

	if req.Address == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ConnectResponse{
			Success: false,
			Error:   "address is required",
		})
		return
	}

	// Parse the multiaddr
	addr, err := multiaddr.NewMultiaddr(req.Address)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ConnectResponse{
			Success: false,
			Error:   "invalid multiaddr: " + err.Error(),
		})
		return
	}

	// Extract peer info from multiaddr
	peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ConnectResponse{
			Success: false,
			Error:   "could not extract peer info: " + err.Error(),
		})
		return
	}

	// Attempt to connect
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	slog.Info("Attempting to connect to peer", "peer_id", peerInfo.ID, "addrs", peerInfo.Addrs)

	if err := s.host.Connect(ctx, *peerInfo); err != nil {
		slog.Error("Failed to connect to peer", "peer_id", peerInfo.ID, "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ConnectResponse{
			Success: false,
			PeerID:  peerInfo.ID.String(),
			Error:   "connection failed: " + err.Error(),
		})
		return
	}

	slog.Info("Successfully connected to peer", "peer_id", peerInfo.ID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ConnectResponse{
		Success: true,
		PeerID:  peerInfo.ID.String(),
	})
}

// Shutdown gracefully shuts down the info server
func (s *InfoServer) Shutdown(ctx context.Context) error {
	if s.partitions != nil {
		s.partitions.Stop()
	}
	if s.metrics != nil {
		s.metrics.Stop()
	}
	return s.server.Shutdown(ctx)
}

// Metrics returns the metrics collector for external use
func (s *InfoServer) Metrics() *MetricsCollector {
	return s.metrics
}

// Partitions returns the partition tracker for external use
func (s *InfoServer) Partitions() *PartitionTracker {
	return s.partitions
}

// SetConnectionManager sets the connection manager
func (s *InfoServer) SetConnectionManager(cm *ConnectionManager) {
	s.connections = cm
}

// Connections returns the connection manager for external use
func (s *InfoServer) Connections() *ConnectionManager {
	return s.connections
}

// multiaddrToStrings converts multiaddrs to strings
func multiaddrToStrings(addrs []multiaddr.Multiaddr) []string {
	result := make([]string, len(addrs))
	for i, addr := range addrs {
		result[i] = addr.String()
	}
	return result
}

// filterLocalAddresses filters out localhost addresses from multiaddr list
func filterLocalAddresses(addrs []multiaddr.Multiaddr) []string {
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		addrStr := addr.String()
		// Skip localhost addresses
		if strings.Contains(addrStr, "/ip4/127.0.0.1/") || strings.Contains(addrStr, "/ip6/::1/") {
			continue
		}
		result = append(result, addrStr)
	}
	return result
}

// buildExternalAddresses builds the full external multiaddrs with peer ID
func buildExternalAddresses(h host.Host, external []multiaddr.Multiaddr) []string {
	peerID := h.ID().String()

	// If external addresses are explicitly configured, use those
	if len(external) > 0 {
		result := make([]string, 0, len(external)*2)

		// Add the configured external addresses with peer ID
		for _, addr := range external {
			addrStr := addr.String()
			if !strings.Contains(addrStr, "/p2p/") {
				addrStr = addrStr + "/p2p/" + peerID
			}
			result = append(result, addrStr)
		}

		// Also add DNS-based variants if we can get the hostname
		hostname := getHostname()
		if hostname != "" {
			// Extract unique ports from external addresses
			ports := make(map[string]bool)
			for _, addr := range external {
				addrStr := addr.String()
				parts := strings.Split(addrStr, "/tcp/")
				if len(parts) == 2 {
					// Remove any trailing /p2p/... suffix
					port := strings.Split(parts[1], "/")[0]
					ports[port] = true
				}
			}

			// Add DNS address for each port
			for port := range ports {
				dnsAddr := fmt.Sprintf("/dns/%s/tcp/%s/p2p/%s", hostname, port, peerID)
				result = append(result, dnsAddr)
			}
		}

		return result
	}

	// Try to get public IP from AWS metadata service
	publicIP := getPublicIP()
	addrs := h.Addrs()

	// If we have a public IP, build proper external addresses
	if publicIP != "" {
		result := make([]string, 0, 2)

		// Extract unique ports from listen addresses
		ports := make(map[string]bool)
		for _, addr := range addrs {
			addrStr := addr.String()
			// Extract port from multiaddr like /ip4/x.x.x.x/tcp/PORT
			parts := strings.Split(addrStr, "/tcp/")
			if len(parts) == 2 {
				ports[parts[1]] = true
			}
		}

		// Build external addresses with public IP for each port
		for port := range ports {
			fullAddr := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", publicIP, port, peerID)
			result = append(result, fullAddr)
		}

		// Also add DNS-based addresses if available
		hostname := getHostname()
		if hostname != "" {
			for port := range ports {
				fullAddr := fmt.Sprintf("/dns/%s/tcp/%s/p2p/%s", hostname, port, peerID)
				result = append(result, fullAddr)
			}
		}

		return result
	}

	// Fallback to local addresses if we can't determine public IP
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		fullAddr := addr.String() + "/p2p/" + peerID
		result = append(result, fullAddr)
	}
	return result
}

// getPublicIP attempts to get the public IP from AWS metadata service
func getPublicIP() string {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://169.254.169.254/latest/meta-data/public-ipv4")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(body))
}

// getHostname attempts to get the public hostname
func getHostname() string {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://169.254.169.254/latest/meta-data/public-hostname")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	hostname := strings.TrimSpace(string(body))

	// If we got an AWS hostname, prefer our custom DNS name
	if strings.Contains(hostname, "compute.amazonaws.com") {
		return "bootstrap.accumulate.defidevs.io"
	}

	return hostname
}

// listenMultiaddr creates a listener from a multiaddr
func listenMultiaddr(addr multiaddr.Multiaddr) (net.Listener, error) {
	// Parse the multiaddr to get the network and address
	parts := multiaddr.Split(addr)
	if len(parts) < 2 {
		return nil, multiaddr.ErrProtocolNotFound
	}

	// Get IP and port
	var ip, port string
	for _, part := range parts {
		protocols := part.Protocols()
		for _, p := range protocols {
			switch p.Code {
			case multiaddr.P_IP4, multiaddr.P_IP6:
				val, err := part.ValueForProtocol(p.Code)
				if err == nil {
					ip = val
				}
			case multiaddr.P_TCP:
				val, err := part.ValueForProtocol(p.Code)
				if err == nil {
					port = val
				}
			}
		}
	}

	if ip == "" || port == "" {
		return nil, multiaddr.ErrProtocolNotFound
	}

	return net.Listen("tcp", ip+":"+port)
}

// handleStats serves detailed statistics
func (s *InfoServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	defer func() {
		s.metrics.RecordHTTPRequest("/stats", http.StatusOK, time.Since(start))
	}()

	stats := s.metrics.GetStats()
	stats["uptime_seconds"] = int64(time.Since(s.startTime).Seconds())
	stats["uptime_hours"] = time.Since(s.startTime).Hours()
	stats["server_start_time"] = s.startTime.Format(time.RFC3339)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(stats); err != nil {
		slog.Error("Failed to encode stats response", "error", err)
	}
}

// handlePeersByPartition serves peers filtered by partition
func (s *InfoServer) handlePeersByPartition(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract partition from path: /peers/{partition}
	path := strings.TrimPrefix(r.URL.Path, "/peers/")
	partition := strings.ToLower(strings.TrimSpace(path))

	if partition == "" {
		http.Error(w, "Partition name required", http.StatusBadRequest)
		return
	}

	start := time.Now()
	defer func() {
		s.metrics.RecordHTTPRequest("/peers/"+partition, http.StatusOK, time.Since(start))
	}()

	// Get peers from partition tracker
	partitionPeers := s.partitions.GetPeersByPartition(partition)

	// Build response with detailed peer info
	peerInfos := make([]map[string]interface{}, 0, len(partitionPeers))
	for _, info := range partitionPeers {
		addrStrs := make([]string, 0, len(info.Addresses))
		for _, addr := range info.Addresses {
			fullAddr := addr.String() + "/p2p/" + info.PeerID.String()
			addrStrs = append(addrStrs, fullAddr)
		}

		peerInfos = append(peerInfos, map[string]interface{}{
			"peer_id":    info.PeerID.String(),
			"addresses":  addrStrs,
			"score":      info.Score,
			"first_seen": info.FirstSeen.Format(time.RFC3339),
			"last_seen":  info.LastSeen.Format(time.RFC3339),
		})
	}

	// If no peers found in tracker, fall back to all connected peers
	if len(peerInfos) == 0 {
		peers := s.host.Network().Peers()
		for _, peerID := range peers {
			addrs := s.host.Peerstore().Addrs(peerID)
			addrStrs := make([]string, 0, len(addrs))
			for _, addr := range addrs {
				fullAddr := addr.String() + "/p2p/" + peerID.String()
				addrStrs = append(addrStrs, fullAddr)
			}

			peerInfos = append(peerInfos, map[string]interface{}{
				"peer_id":   peerID.String(),
				"addresses": addrStrs,
				"note":      "partition not yet determined",
			})
		}
	}

	response := map[string]interface{}{
		"partition": partition,
		"count":     len(peerInfos),
		"peers":     peerInfos,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode peers by partition response", "error", err)
	}
}

// handlePartitions lists known partitions and their peer counts
func (s *InfoServer) handlePartitions(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	defer func() {
		s.metrics.RecordHTTPRequest("/partitions", http.StatusOK, time.Since(start))
	}()

	// Get partition stats from partition tracker
	partitionStats := s.partitions.GetPartitionStats()

	// Build response with detailed partition info
	partitions := make(map[string]interface{})
	for name, stats := range partitionStats {
		partitions[name] = map[string]interface{}{
			"total_peers":     stats.TotalPeers,
			"connected_peers": stats.ConnectedPeers,
			"average_score":   stats.AverageScore,
		}
	}

	// Ensure known partitions are present even if empty
	if _, ok := partitions[PartitionDN]; !ok {
		partitions[PartitionDN] = map[string]interface{}{
			"total_peers":     0,
			"connected_peers": 0,
			"average_score":   0.0,
		}
	}
	if _, ok := partitions[PartitionCyclops]; !ok {
		partitions[PartitionCyclops] = map[string]interface{}{
			"total_peers":     0,
			"connected_peers": 0,
			"average_score":   0.0,
		}
	}

	response := map[string]interface{}{
		"partitions":            partitions,
		"total_connected_peers": len(s.host.Network().Peers()),
		"total_tracked_peers":   len(s.partitions.GetAllPeers()),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode partitions response", "error", err)
	}
}

// handleDebugDHT provides debugging information about the DHT routing table
func (s *InfoServer) handleDebugDHT(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	defer func() {
		s.metrics.RecordHTTPRequest("/debug/dht", http.StatusOK, time.Since(start))
	}()

	peers := s.host.Network().Peers()
	conns := s.host.Network().Conns()

	// Gather detailed peer information
	peerDetails := make([]map[string]interface{}, 0, len(peers))
	for _, peerID := range peers {
		addrs := s.host.Peerstore().Addrs(peerID)
		protocols, _ := s.host.Peerstore().GetProtocols(peerID)

		// Check if connected
		connected := false
		var connDirection string
		for _, conn := range conns {
			if conn.RemotePeer() == peerID {
				connected = true
				if conn.Stat().Direction == network.DirInbound {
					connDirection = "inbound"
				} else {
					connDirection = "outbound"
				}
				break
			}
		}

		peerDetails = append(peerDetails, map[string]interface{}{
			"peer_id":    peerID.String(),
			"addresses":  multiaddrToStrings(addrs),
			"protocols":  protocols,
			"connected":  connected,
			"direction":  connDirection,
		})
	}

	response := map[string]interface{}{
		"self_peer_id":       s.host.ID().String(),
		"routing_table_size": len(peers),
		"connection_count":   len(conns),
		"peers":              peerDetails,
		"listen_addresses":   filterLocalAddresses(s.host.Addrs()),
		"external_addresses": buildExternalAddresses(s.host, s.external),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode debug DHT response", "error", err)
	}
}

// handleConnections provides connection management statistics
func (s *InfoServer) handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	defer func() {
		s.metrics.RecordHTTPRequest("/connections", http.StatusOK, time.Since(start))
	}()

	var response map[string]interface{}

	if s.connections != nil {
		response = s.connections.GetConnectionStats()
	} else {
		// Fallback if connection manager not set
		conns := s.host.Network().Conns()
		inbound := 0
		outbound := 0
		for _, conn := range conns {
			if conn.Stat().Direction == network.DirInbound {
				inbound++
			} else {
				outbound++
			}
		}
		response = map[string]interface{}{
			"total":        len(conns),
			"inbound":      inbound,
			"outbound":     outbound,
			"unique_peers": len(s.host.Network().Peers()),
			"note":         "connection manager not enabled",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode connections response", "error", err)
	}
}
