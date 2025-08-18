//go:build ignore
// +build ignore

// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func main() {
	fmt.Println("🔍 Testing Direct Query Methods for DN Height")
	fmt.Println("==============================================")
	
	// Since the DN height is stored in the AnchorPool, let's try different query approaches
	endpoints := []string{
		"https://mainnet.accumulatenetwork.io/v3",
		"https://apollo-mainnet.accumulate.defidevs.io/v3",
		"http://apollo-mainnet.accumulate.defidevs.io:16695/v3",
	}
	
	for _, endpoint := range endpoints {
		fmt.Printf("\n📡 Testing endpoint: %s\n", endpoint)
		
		// Test 1: Query with includeRemote
		fmt.Println("\n  1. Query with includeRemote=true:")
		resp := makeRequest(endpoint, "query", map[string]interface{}{
			"scope": "acc://dn.acme/anchors",
			"includeRemote": true,
		})
		
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				fmt.Printf("    Result type: %v\n", result["type"])
				if records, ok := result["records"].([]interface{}); ok {
					fmt.Printf("    Records found: %d\n", len(records))
				}
			}
		}
		
		// Test 2: Query directory metrics
		fmt.Println("\n  2. Query metrics for Directory partition:")
		resp = makeRequest(endpoint, "metrics", map[string]interface{}{
			"partition": "Directory",
		})
		
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				fmt.Printf("    Metrics: %v\n", result)
			}
		}
		
		// Test 3: Query consensus status if available
		fmt.Println("\n  3. Query consensus status:")
		resp = makeRequest(endpoint, "consensus-status", map[string]interface{}{
			"partition": "Directory",
		})
		
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				fmt.Printf("    Consensus: %v\n", result)
			}
		}
		
		// Test 4: Try query-tx for anchor transactions
		fmt.Println("\n  4. Query recent transactions:")
		resp = makeRequest(endpoint, "query-tx", map[string]interface{}{
			"scope": "acc://dn.acme",
			"query": map[string]interface{}{
				"queryType": "default",
				"includeRemote": false,
			},
		})
		
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				if records, ok := result["records"].([]interface{}); ok {
					fmt.Printf("    Found %d transactions\n", len(records))
				}
			}
		}
	}
	
	// Let's also try the CometBFT RPC to get blockchain info
	fmt.Println("\n📡 Testing CometBFT RPC for blockchain info:")
	
	// Get the latest block from apollo
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://apollo-mainnet.accumulate.defidevs.io:16692/block")
	if err == nil && resp != nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var blockInfo map[string]interface{}
		if json.Unmarshal(body, &blockInfo) == nil {
			if result, ok := blockInfo["result"].(map[string]interface{}); ok {
				if block, ok := result["block"].(map[string]interface{}); ok {
					if header, ok := block["header"].(map[string]interface{}); ok {
						fmt.Printf("  Latest block height: %v\n", header["height"])
						fmt.Printf("  Chain ID: %v\n", header["chain_id"])
						fmt.Printf("  Time: %v\n", header["time"])
						
						// Check if there's app_hash or other data that might contain DN info
						if appHash, ok := header["app_hash"].(string); ok && appHash != "" {
							fmt.Printf("  App Hash: %s\n", appHash)
						}
					}
				}
			}
		}
	}
	
	// Monitor for a bit to see if anything changes
	fmt.Println("\n⏰ Monitoring for changes (10 seconds):")
	
	prevHeights := make(map[string]float64)
	
	for i := 0; i < 5; i++ {
		fmt.Printf("\n  Check %d at %s:\n", i+1, time.Now().Format("15:04:05"))
		
		// Check network-status on each endpoint
		for _, endpoint := range endpoints {
			resp := makeRequest(endpoint, "network-status", map[string]interface{}{})
			if resp != nil {
				if result, ok := resp["result"].(map[string]interface{}); ok {
					if dirHeight, ok := result["directoryHeight"].(float64); ok {
						prev := prevHeights[endpoint]
						if prev > 0 && dirHeight != prev {
							fmt.Printf("    🎯 %s: %.0f → %.0f CHANGED!\n", endpoint, prev, dirHeight)
						} else {
							fmt.Printf("    %s: %.0f\n", endpoint, dirHeight)
						}
						prevHeights[endpoint] = dirHeight
					}
				}
			}
		}
		
		// Also check CometBFT block height
		resp, err := client.Get("http://apollo-mainnet.accumulate.defidevs.io:16692/status")
		if err == nil && resp != nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var status map[string]interface{}
			if json.Unmarshal(body, &status) == nil {
				if result, ok := status["result"].(map[string]interface{}); ok {
					if syncInfo, ok := result["sync_info"].(map[string]interface{}); ok {
						if height, ok := syncInfo["latest_block_height"].(string); ok {
							fmt.Printf("    Cyclops BVN: %s (live)\n", height)
						}
					}
				}
			}
		}
		
		if i < 4 {
			time.Sleep(2 * time.Second)
		}
	}
}

func makeRequest(endpoint, method string, params interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}
	
	jsonData, _ := json.Marshal(payload)
	
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	
	return result
}