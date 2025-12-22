package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SyncProgressResult contains sync progress information
type SyncProgressResult struct {
	Status          string  `json:"status"`           // syncing, synced, stalled, unknown
	LocalHeight     int64   `json:"local_height"`     // Current block height on follower
	NetworkHeight   int64   `json:"network_height"`   // Current block height on network
	BlocksBehind    int64   `json:"blocks_behind"`    // Difference
	SyncPercentage  float64 `json:"sync_percentage"`  // 0-100
	EstimatedETA    string  `json:"estimated_eta"`    // Human-readable ETA
	SyncRateBlocks  float64 `json:"sync_rate_blocks"` // Blocks per minute
	PeerCount       int     `json:"peer_count"`       // Number of connected peers
	DNHeight        int64   `json:"dn_height"`        // DN specific height
	BVNHeight       int64   `json:"bvn_height"`       // BVN specific height
	ContainerStatus string  `json:"container_status"` // running, stopped, not_found
	LastBlockTime   string  `json:"last_block_time"`  // Timestamp of last block
	Message         string  `json:"message"`          // Human-readable summary
}

// getSyncProgress returns detailed sync progress for a running follower
func (s *Server) getSyncProgress(args map[string]interface{}) (map[string]interface{}, error) {
	containerName, _ := args["container_name"].(string)
	if containerName == "" {
		containerName = "accumulate-follower"
	}

	// Validate container name to prevent command injection
	if err := ValidateContainerName(containerName); err != nil {
		return nil, fmt.Errorf("invalid container_name: %w", err)
	}

	includeRate, _ := args["include_rate"].(bool)

	result := &SyncProgressResult{
		Status: "unknown",
	}

	// Check container status
	running, err := isContainerRunning(containerName)
	if err != nil {
		result.ContainerStatus = "error"
		result.Message = fmt.Sprintf("Error checking container: %v", err)
		return structToMap(result)
	}

	if !running {
		exists, _ := containerExists(containerName)
		if exists {
			result.ContainerStatus = "stopped"
			result.Status = "stopped"
			result.Message = "Follower container exists but is not running"
		} else {
			result.ContainerStatus = "not_found"
			result.Status = "not_found"
			result.Message = "Follower container not found"
		}
		return structToMap(result)
	}

	result.ContainerStatus = "running"

	// Get local node heights from container logs or API
	dnHeight, bvnHeight, peerCount, lastBlockTime := getLocalNodeInfo(containerName)
	result.DNHeight = dnHeight
	result.BVNHeight = bvnHeight
	result.PeerCount = peerCount
	result.LastBlockTime = lastBlockTime

	// Use the higher of DN/BVN as local height
	if dnHeight > bvnHeight {
		result.LocalHeight = dnHeight
	} else {
		result.LocalHeight = bvnHeight
	}

	// Get network height from mainnet
	networkHeight, err := getNetworkHeight("mainnet")
	if err != nil {
		result.Message = fmt.Sprintf("Could not fetch network height: %v", err)
		result.Status = "unknown"
	} else {
		result.NetworkHeight = networkHeight
		result.BlocksBehind = networkHeight - result.LocalHeight

		if result.BlocksBehind <= 0 {
			result.SyncPercentage = 100.0
			result.Status = "synced"
			result.BlocksBehind = 0
			result.EstimatedETA = "Synced"
			result.Message = "Follower is fully synced with the network"
		} else if result.BlocksBehind < 100 {
			result.SyncPercentage = float64(result.LocalHeight) / float64(networkHeight) * 100
			result.Status = "synced"
			result.EstimatedETA = "< 1 minute"
			result.Message = fmt.Sprintf("Follower is nearly synced, %d blocks behind", result.BlocksBehind)
		} else {
			result.SyncPercentage = float64(result.LocalHeight) / float64(networkHeight) * 100
			result.Status = "syncing"
			result.Message = fmt.Sprintf("Syncing: %d blocks behind (%.2f%%)", result.BlocksBehind, result.SyncPercentage)
		}
	}

	// Calculate sync rate if requested
	if includeRate && result.Status == "syncing" {
		rate := calculateSyncRate(containerName)
		result.SyncRateBlocks = rate
		if rate > 0 {
			minutesToSync := float64(result.BlocksBehind) / rate
			if minutesToSync < 60 {
				result.EstimatedETA = fmt.Sprintf("%.0f minutes", minutesToSync)
			} else if minutesToSync < 1440 {
				result.EstimatedETA = fmt.Sprintf("%.1f hours", minutesToSync/60)
			} else {
				result.EstimatedETA = fmt.Sprintf("%.1f days", minutesToSync/1440)
			}
		} else {
			result.EstimatedETA = "Unknown (calculating rate...)"
		}
	}

	// Check for stalled condition
	if result.Status == "syncing" && result.PeerCount == 0 {
		result.Status = "stalled"
		result.Message = "Sync stalled: no peers connected"
	}

	return structToMap(result)
}

// getLocalNodeInfo extracts node information from container logs or API
func getLocalNodeInfo(containerName string) (dnHeight, bvnHeight int64, peerCount int, lastBlockTime string) {
	// Try to get info from container logs (last 100 lines)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", "100", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, 0, ""
	}

	logs := string(output)

	// Parse heights from logs
	// Look for patterns like "height=12345" or "block height: 12345"
	heightRegex := regexp.MustCompile(`(?i)height[=:\s]+(\d+)`)
	matches := heightRegex.FindAllStringSubmatch(logs, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			if h, err := strconv.ParseInt(match[1], 10, 64); err == nil {
				if h > dnHeight {
					dnHeight = h
				}
			}
		}
	}

	// Look for peer count
	peerRegex := regexp.MustCompile(`(?i)peers?[=:\s]+(\d+)`)
	peerMatches := peerRegex.FindAllStringSubmatch(logs, -1)
	for _, match := range peerMatches {
		if len(match) >= 2 {
			if p, err := strconv.Atoi(match[1]); err == nil && p > peerCount {
				peerCount = p
			}
		}
	}

	// Try to get more accurate info from the node API if accessible
	// DN API typically on port 16592
	dnAPIHeight := queryNodeAPIHeight("http://localhost:16592")
	if dnAPIHeight > dnHeight {
		dnHeight = dnAPIHeight
	}

	// BVN API typically on port 16692
	bvnAPIHeight := queryNodeAPIHeight("http://localhost:16692")
	if bvnAPIHeight > bvnHeight {
		bvnHeight = bvnAPIHeight
	}

	return dnHeight, bvnHeight, peerCount, lastBlockTime
}

// queryNodeAPIHeight queries a node's API for current height
func queryNodeAPIHeight(baseURL string) int64 {
	client := &http.Client{Timeout: 5 * time.Second}

	// Try the /status endpoint
	resp, err := client.Get(baseURL + "/status")
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0
	}

	// Navigate the response structure
	if syncInfo, ok := result["sync_info"].(map[string]interface{}); ok {
		if latestHeight, ok := syncInfo["latest_block_height"].(string); ok {
			if h, err := strconv.ParseInt(latestHeight, 10, 64); err == nil {
				return h
			}
		}
		if latestHeight, ok := syncInfo["latest_block_height"].(float64); ok {
			return int64(latestHeight)
		}
	}

	return 0
}

// getNetworkHeight fetches the current network height from mainnet
func getNetworkHeight(network string) (int64, error) {
	var endpoint string
	switch network {
	case "mainnet":
		endpoint = "https://mainnet.accumulatenetwork.io/v2"
	case "testnet":
		endpoint = "https://testnet.accumulatenetwork.io/v2"
	default:
		endpoint = network
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Query network status
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"network-status","params":{}}`
	resp, err := client.Post(endpoint, "application/json", strings.NewReader(reqBody))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	// Parse the response to get block height
	if res, ok := result["result"].(map[string]interface{}); ok {
		if directoryHeight, ok := res["directoryHeight"].(float64); ok {
			return int64(directoryHeight), nil
		}
		// Try alternate path
		if status, ok := res["status"].(map[string]interface{}); ok {
			if height, ok := status["height"].(float64); ok {
				return int64(height), nil
			}
		}
	}

	// Fallback: try querying a specific account's latest block
	return queryLatestBlockHeight(endpoint)
}

// queryLatestBlockHeight gets the latest block height by querying a system account
func queryLatestBlockHeight(endpoint string) (int64, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Query the DN system account
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"query","params":{"url":"acc://dn.acme"}}`
	resp, err := client.Post(endpoint, "application/json", strings.NewReader(reqBody))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	// Look for lastBlockTime or height in the response
	if res, ok := result["result"].(map[string]interface{}); ok {
		if mainChain, ok := res["mainChain"].(map[string]interface{}); ok {
			if height, ok := mainChain["height"].(float64); ok {
				return int64(height), nil
			}
		}
	}

	return 0, fmt.Errorf("could not determine network height")
}

// calculateSyncRate calculates the sync rate by comparing heights over time
func calculateSyncRate(containerName string) float64 {
	// Get current height
	dnHeight1, _, _, _ := getLocalNodeInfo(containerName)
	if dnHeight1 == 0 {
		return 0
	}

	// Wait 10 seconds
	time.Sleep(10 * time.Second)

	// Get new height
	dnHeight2, _, _, _ := getLocalNodeInfo(containerName)
	if dnHeight2 == 0 {
		return 0
	}

	// Calculate rate (blocks per minute)
	blocksDiff := dnHeight2 - dnHeight1
	if blocksDiff <= 0 {
		return 0
	}

	// Rate = blocks / (10 seconds) * 60 = blocks per minute
	return float64(blocksDiff) * 6.0
}

// structToMap converts a struct to a map[string]interface{}
func structToMap(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}
