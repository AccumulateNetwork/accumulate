package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// RootHashMonitor monitors the protocol's root hash for changes
type RootHashMonitor struct {
	serverURL    string
	httpClient   *http.Client
	lastRootHash string
	lastUpdate   time.Time
}

// NewRootHashMonitor creates a new root hash monitor instance
func NewRootHashMonitor(serverURL string) (*RootHashMonitor, error) {
	// Normalize server URL with fallback endpoints
	switch serverURL {
	case "local":
		serverURL = "http://127.0.1.1:26660"
	case "testnet":
		serverURL = "https://testnet.accumulatenetwork.io"
	case "beta":
		serverURL = "https://beta.testnet.accumulatenetwork.io"
	case "canary":
		serverURL = "https://canary.testnet.accumulatenetwork.io"
	case "", "mainnet":
		// Based on troubleshooting doc, try direct node access first
		serverURL = "http://apollo-mainnet.accumulate.defidevs.io:16595"
	case "mainnet-ssl":
		serverURL = "https://mainnet.accumulatenetwork.io"
	}

	// Try v3 endpoint first (current API)
	if !strings.HasSuffix(serverURL, "/v3") && !strings.HasSuffix(serverURL, "/v2") {
		if strings.HasSuffix(serverURL, "/") {
			serverURL += "v3"
		} else {
			serverURL += "/v3"
		}
	}

	return &RootHashMonitor{
		serverURL: serverURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// MonitorRootHash continuously monitors the protocol's root hash
func (rhm *RootHashMonitor) MonitorRootHash(ctx context.Context) error {
	fmt.Printf("Starting root hash monitor for: %s\n", rhm.serverURL)
	fmt.Printf("Checking every 1 second for changes...\n\n")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nRoot hash monitoring stopped")
			return ctx.Err()
		case <-ticker.C:
			if err := rhm.checkRootHash(ctx); err != nil {
				log.Printf("Error checking root hash: %v", err)
			}
		}
	}
}

// checkRootHash queries the current root hash and logs changes
func (rhm *RootHashMonitor) checkRootHash(ctx context.Context) error {
	// Query the protocol's root hash
	query := map[string]interface{}{
		"scope": "dn.acme",
	}

	resp, err := rhm.sendQuery(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query protocol: %w", err)
	}

	// Extract root hash from response
	rootHash, err := rhm.extractRootHash(resp)
	if err != nil {
		return fmt.Errorf("failed to extract root hash: %w", err)
	}

	currentTime := time.Now()

	// Check if root hash has changed
	if rhm.lastRootHash == "" {
		// First time - just log the initial hash
		fmt.Printf("[%s] Initial root hash: %s\n", 
			currentTime.Format("2006-01-02 15:04:05"), rootHash)
		rhm.lastRootHash = rootHash
		rhm.lastUpdate = currentTime
	} else if rhm.lastRootHash != rootHash {
		// Root hash changed - log the change
		timeSinceLastChange := currentTime.Sub(rhm.lastUpdate)
		fmt.Printf("[%s] ROOT HASH CHANGED!\n", currentTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Previous: %s\n", rhm.lastRootHash)
		fmt.Printf("  Current:  %s\n", rootHash)
		fmt.Printf("  Time since last change: %v\n\n", timeSinceLastChange)
		
		rhm.lastRootHash = rootHash
		rhm.lastUpdate = currentTime
	}
	// If no change, we silently continue (no spam logging)

	return nil
}

// extractRootHash extracts the root hash from the protocol response
func (rhm *RootHashMonitor) extractRootHash(resp map[string]interface{}) (string, error) {
	// Look for root hash in the response
	// The exact field name may vary, so we'll check common locations
	
	// Check if there's a direct rootHash field
	if rootHash, ok := resp["rootHash"].(string); ok {
		return rootHash, nil
	}

	// Check in account field
	if account, ok := resp["account"].(map[string]interface{}); ok {
		if rootHash, ok := account["rootHash"].(string); ok {
			return rootHash, nil
		}
	}

	// Check in chains field for the root chain
	if chains, ok := resp["chains"].(map[string]interface{}); ok {
		if rootChain, ok := chains["root"].(map[string]interface{}); ok {
			if rootHash, ok := rootChain["rootHash"].(string); ok {
				return rootHash, nil
			}
		}
	}

	// Check for merkle root or similar fields
	if merkleRoot, ok := resp["merkleRoot"].(string); ok {
		return merkleRoot, nil
	}

	// If we can't find a root hash, return the entire response as JSON for debugging
	jsonResp, _ := json.MarshalIndent(resp, "", "  ")
	return "", fmt.Errorf("could not find root hash in response: %s", string(jsonResp))
}

// sendQuery sends a JSON-RPC 2.0 query request to the Accumulate API
func (rhm *RootHashMonitor) sendQuery(ctx context.Context, query map[string]interface{}) (map[string]interface{}, error) {
	// Create JSON-RPC 2.0 request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "query",
		"params":  query,
		"id":      1,
	}

	// Marshal request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", rhm.serverURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := rhm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON-RPC response
	var jsonResp map[string]interface{}
	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Check for JSON-RPC error
	if errField, ok := jsonResp["error"]; ok {
		return nil, fmt.Errorf("JSON-RPC error: %v", errField)
	}

	// Extract result
	result, ok := jsonResp["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no result field in response")
	}

	return result, nil
}
