// Copyright 2025 The Accumulate Authors
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

// InfoServer serves bootstrap server information on HTTP
type InfoServer struct {
	host      host.Host
	server    *http.Server
	startTime time.Time
	external  []multiaddr.Multiaddr
}

// NewInfoServer creates a new info server
func NewInfoServer(h host.Host, listen multiaddr.Multiaddr, external []multiaddr.Multiaddr) (*InfoServer, error) {
	s := &InfoServer{
		host:      h,
		startTime: time.Now(),
		external:  external,
	}

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/info", s.handleInfo)
	mux.HandleFunc("/health", s.handleHealth)

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

	// Check if we have any connections or peers
	peers := s.host.Network().Peers()
	conns := s.host.Network().Conns()

	status := HealthStatus{
		Status: "healthy",
	}

	// Consider unhealthy if no peers after 5 minutes
	if len(peers) == 0 && time.Since(s.startTime) > 5*time.Minute {
		status.Status = "unhealthy"
		status.Reason = "no peers in DHT routing table after 5 minutes"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}

	// Include some basic stats in health check
	healthDetails := map[string]interface{}{
		"status":       status.Status,
		"peer_count":   len(peers),
		"conn_count":   len(conns),
		"uptime_hours": int64(time.Since(s.startTime).Hours()),
	}
	if status.Reason != "" {
		healthDetails["reason"] = status.Reason
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(healthDetails); err != nil {
		slog.Error("Failed to encode health response", "error", err)
	}
}

// Shutdown gracefully shuts down the info server
func (s *InfoServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
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
