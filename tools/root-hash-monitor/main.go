package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// RootHashMonitor monitors the Accumulate network for root hash changes
type RootHashMonitor struct {
	Endpoint     string
	httpClient   *http.Client
	LastRootHash string
	LastTime     time.Time
	verbose      bool
	client       *http.Client
	dnEndpoint   string
	bvnEndpoint  string
}

// JSONRPCRequest represents a JSON-RPC v3 request
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

// JSONRPCResponse represents a JSON-RPC v3 response
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      int             `json:"id"`
}

// JSONRPCError represents a JSON-RPC error
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ChainQueryResponse represents the response from querying a chain
type ChainQueryResponse struct {
	Type    string `json:"type"`
	Records []struct {
		Entry struct {
			Hash [32]byte `json:"hash"`
		} `json:"entry"`
		Value struct {
			Transaction struct {
				Body json.RawMessage `json:"body"`
			} `json:"transaction"`
		} `json:"value"`
	} `json:"records"`
}

// DirectoryAnchor represents a directory anchor transaction body
type DirectoryAnchor struct {
	Type            string    `json:"type"`
	Source          string    `json:"source"`
	MajorBlockIndex uint64    `json:"majorBlockIndex"`
	MinorBlockIndex uint64    `json:"minorBlockIndex"`
	RootChainIndex  uint64    `json:"rootChainIndex"`
	RootChainAnchor [32]byte  `json:"rootChainAnchor"`
	StateTreeAnchor [32]byte  `json:"stateTreeAnchor"`
	Updates         []interface{} `json:"updates,omitempty"`
	Receipts        []interface{} `json:"receipts,omitempty"`
	MakeMajorBlock  uint64    `json:"makeMajorBlock,omitempty"`
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

	// Ensure v3 endpoint
	if serverURL[len(serverURL)-1] == '/' {
		serverURL += "v3"
	} else {
		serverURL += "/v3"
	}

	return &RootHashMonitor{
		Endpoint: serverURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		dnEndpoint: serverURL,
		bvnEndpoint: serverURL,
	}, nil
}

// MonitorRootHash continuously monitors the protocol's root hash
func (rhm *RootHashMonitor) MonitorRootHash(ctx context.Context) error {
	fmt.Printf("Starting root hash monitor for: %s\n", rhm.Endpoint)
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
	// Monitor DN root hash
	dnHash, err := rhm.getRootHash(rhm.dnEndpoint, "acc://dn.acme")
	if err != nil {
		log.Printf("Error getting DN root hash: %v", err)
		return err
	}

	// Monitor BVN root hash
	bvnHash, err := rhm.getRootHash(rhm.bvnEndpoint, "acc://bvn-cyclops.acme")
	if err != nil {
		log.Printf("Error getting BVN root hash: %v", err)
		return err
	}

	// Compare DN and BVN root hashes for consistency
	dnHashStr := hex.EncodeToString(dnHash[:])
	bvnHashStr := hex.EncodeToString(bvnHash[:])
	
	if dnHashStr != bvnHashStr {
		log.Printf("[%s] WARNING: Root hash mismatch between partitions!", time.Now().Format("2006-01-02 15:04:05"))
		log.Printf("  DN Hash:  %s", dnHashStr)
		log.Printf("  BVN Hash: %s", bvnHashStr)
	}

	// Check if this is the first time (no previous hash)
	if rhm.LastRootHash == "" {
		// First time - just store the DN hash as reference
		rhm.LastRootHash = dnHashStr
		rhm.LastTime = time.Now()
		log.Printf("[%s] Initial DN root hash: %s", time.Now().Format("2006-01-02 15:04:05"), rhm.LastRootHash)
		log.Printf("[%s] Initial BVN root hash: %s", time.Now().Format("2006-01-02 15:04:05"), bvnHashStr)
		return nil
	}

	// Check if the DN root hash has changed
	if dnHashStr != rhm.LastRootHash {
		now := time.Now()
		timeSinceLastChange := now.Sub(rhm.LastTime)

		log.Printf("[%s] DN ROOT HASH CHANGED!", now.Format("2006-01-02 15:04:05"))
		log.Printf("  Previous: %s", rhm.LastRootHash)
		log.Printf("  Current:  %s", dnHashStr)
		log.Printf("  BVN Hash: %s", bvnHashStr)
		log.Printf("  Time since last change: %v", timeSinceLastChange)

		// Update stored values
		rhm.LastRootHash = dnHashStr
		rhm.LastTime = now
	}

	return nil
}

// getRootHash retrieves the current BPT root hash from a partition by querying the latest anchor transaction
func (m *RootHashMonitor) getRootHash(endpoint string, partition string) ([32]byte, error) {
	var zeroHash [32]byte

	// Strategy: Query the anchor chain to get the latest anchor transaction
	// The StateTreeAnchor field contains the BPT root hash
	
	// First, determine the anchor chain URL based on partition
	var anchorChainURL string
	if partition == "acc://dn.acme" {
		// For Directory Node, query the anchor chain
		anchorChainURL = "acc://dn.acme/anchors"
	} else {
		// For BVN, query the anchor chain (assuming bvn-cyclops for now)
		anchorChainURL = "acc://bvn-cyclops.acme/anchors"
	}

	// Create the JSON-RPC request to query the anchor chain
	reqParams := map[string]interface{}{
		"scope": anchorChainURL,
		"query": map[string]interface{}{
			"type": "chain",
			"range": map[string]interface{}{
				"start": -1, // Get the latest entry
				"count": 1,
			},
		},
	}

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "query",
		Params:  reqParams,
		ID:      1,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return zeroHash, fmt.Errorf("marshal request: %w", err)
	}

	if m.verbose {
		log.Printf("Querying anchor chain %s with request: %s", anchorChainURL, string(reqBody))
	}

	resp, err := m.client.Post(endpoint, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return zeroHash, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	var jsonResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		return zeroHash, fmt.Errorf("decode response: %w", err)
	}

	if jsonResp.Error != nil {
		return zeroHash, fmt.Errorf("JSON-RPC error: %s", jsonResp.Error.Message)
	}

	if m.verbose {
		log.Printf("Anchor chain response: %s", string(jsonResp.Result))
	}

	// Parse the chain query response
	var chainResp ChainQueryResponse
	if err := json.Unmarshal(jsonResp.Result, &chainResp); err != nil {
		return zeroHash, fmt.Errorf("parse chain response: %w", err)
	}

	// Check if we have any records
	if len(chainResp.Records) == 0 {
		return zeroHash, fmt.Errorf("no anchor transactions found in chain %s", anchorChainURL)
	}

	// Get the latest anchor transaction (first record since we requested start=-1)
	latestRecord := chainResp.Records[0]
	
	// Parse the transaction body as DirectoryAnchor
	var anchor DirectoryAnchor
	if err := json.Unmarshal(latestRecord.Value.Transaction.Body, &anchor); err != nil {
		return zeroHash, fmt.Errorf("parse anchor transaction body: %w", err)
	}

	if m.verbose {
		log.Printf("Found anchor: Source=%s, MajorBlock=%d, MinorBlock=%d, StateTreeAnchor=%x", 
			anchor.Source, anchor.MajorBlockIndex, anchor.MinorBlockIndex, anchor.StateTreeAnchor)
	}

	// Return the StateTreeAnchor (BPT root hash)
	return anchor.StateTreeAnchor, nil
}

// main function
func main() {
	// Check command line arguments
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <network>")
		fmt.Println("Networks: mainnet, testnet, beta, canary, local")
		os.Exit(1)
	}

	network := os.Args[1]

	// Create root hash monitor
	monitor, err := NewRootHashMonitor(network)
	if err != nil {
		log.Fatalf("Failed to create monitor: %v", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start monitoring in a goroutine
	go func() {
		if err := monitor.MonitorRootHash(ctx); err != nil && err != context.Canceled {
			log.Printf("Monitor error: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-sigChan
	fmt.Println("\nShutdown signal received, stopping monitor...")
	cancel()

	// Give some time for graceful shutdown
	time.Sleep(100 * time.Millisecond)
	fmt.Println("Monitor stopped.")
}
