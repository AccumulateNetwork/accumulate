package server

import (
	"fmt"
	"time"
)

// GetBootstrapServerStatus queries and returns comprehensive bootstrap server status
func (s *Server) getBootstrapServerStatus(args map[string]interface{}) (map[string]interface{}, error) {
	serverURL, _ := args["server_url"].(string)
	if serverURL == "" {
		serverURL = "http://bootstrap.accumulate.defidevs.io:8080"
	}

	client := NewBootstrapClient()

	// Query all endpoints
	info, infoErr := client.QueryBootstrapServerInfo(serverURL)
	health, healthErr := client.QueryBootstrapServerHealth(serverURL)
	peers, peersErr := client.QueryBootstrapServerPeers(serverURL)

	result := map[string]interface{}{
		"server_url":  serverURL,
		"query_time":  time.Now().Format(time.RFC3339),
	}

	// Add info data
	if infoErr != nil {
		result["info_error"] = infoErr.Error()
		result["info_status"] = "error"
	} else {
		result["info_status"] = "ok"
		result["info"] = map[string]interface{}{
			"peer_id":            info.PeerID,
			"listen_addresses":   info.ListenAddresses,
			"external_addresses": info.ExternalAddresses,
			"dht": map[string]interface{}{
				"mode":               info.DHT.Mode,
				"routing_table_size": info.DHT.RoutingTableSize,
			},
			"connections": map[string]interface{}{
				"total":    info.Connections.Total,
				"inbound":  info.Connections.Inbound,
				"outbound": info.Connections.Outbound,
			},
			"uptime_seconds": info.UptimeSeconds,
			"uptime_hours":   float64(info.UptimeSeconds) / 3600.0,
		}
	}

	// Add health data
	if healthErr != nil {
		result["health_error"] = healthErr.Error()
		result["health_status"] = "error"
	} else {
		result["health_status"] = health.Status
		result["health"] = map[string]interface{}{
			"status":       health.Status,
			"peer_count":   health.PeerCount,
			"conn_count":   health.ConnCount,
			"uptime_hours": health.UptimeHours,
		}
		if health.Reason != "" {
			result["health"].(map[string]interface{})["reason"] = health.Reason
		}
	}

	// Add peers data
	if peersErr != nil {
		result["peers_error"] = peersErr.Error()
		result["peers_status"] = "error"
	} else {
		result["peers_status"] = "ok"
		peerList := make([]map[string]interface{}, len(peers.Peers))
		for i, peer := range peers.Peers {
			peerList[i] = map[string]interface{}{
				"peer_id":   peer.PeerID,
				"addresses": peer.Addresses,
			}
		}
		result["peers"] = map[string]interface{}{
			"count": peers.Count,
			"list":  peerList,
		}
	}

	// Overall status summary
	overallHealthy := true
	var issues []string

	if infoErr != nil {
		overallHealthy = false
		issues = append(issues, "Failed to query /info endpoint")
	}
	if healthErr != nil {
		overallHealthy = false
		issues = append(issues, "Failed to query /health endpoint")
	} else if health.Status != "healthy" {
		overallHealthy = false
		issues = append(issues, fmt.Sprintf("Health status: %s", health.Status))
		if health.Reason != "" {
			issues = append(issues, health.Reason)
		}
	}
	if peersErr != nil {
		// Peers endpoint might not exist on older versions, don't fail on this
		issues = append(issues, "Note: /peers endpoint not available (may be running older version)")
	}

	if overallHealthy {
		result["overall_status"] = "healthy"
		result["message"] = "Bootstrap server is operational"
	} else {
		result["overall_status"] = "unhealthy"
		result["message"] = "Bootstrap server has issues"
		result["issues"] = issues
	}

	return result, nil
}
