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
	fmt.Println("🔍 Testing Block Queries with Correct Parameters")
	fmt.Println("=================================================")
	
	// Test 1: Query specific minor block
	fmt.Println("\n📊 Test 1: Query latest minor blocks")
	testLatestMinorBlocks()
	
	// Test 2: Query minor block range
	fmt.Println("\n📊 Test 2: Query minor block range")
	testMinorBlockRange()
	
	// Test 3: Monitor changes
	fmt.Println("\n⏰ Test 3: Monitor minor block changes (20 seconds)")
	monitorBlocks()
}

func testLatestMinorBlocks() {
	// First get current network status to know approximate block height
	resp := makeRequest("network-status", map[string]interface{}{})
	var currentHeight float64 = 2460315 // Default from what we've seen
	
	if result, ok := resp["result"].(map[string]interface{}); ok {
		if dirHeight, ok := result["directoryHeight"].(float64); ok {
			currentHeight = dirHeight
			fmt.Printf("  Current directory height from network-status: %.0f\n", currentHeight)
		}
	}
	
	// Now query a specific minor block
	blockNum := uint64(currentHeight)
	resp = makeRequest("query", map[string]interface{}{
		"scope": "acc://dn.acme",
		"query": map[string]interface{}{
			"queryType": "block",
			"minor": blockNum,
		},
	})
	
	if result, ok := resp["result"].(map[string]interface{}); ok {
		fmt.Printf("  Query result type: %v\n", result["type"])
		if value, ok := result["value"].(map[string]interface{}); ok {
			fmt.Printf("  Block info: Index=%v, Source=%v\n", value["index"], value["source"])
			if entries, ok := value["entries"].([]interface{}); ok {
				fmt.Printf("  Block has %d entries\n", len(entries))
			}
		}
	}
}

func testMinorBlockRange() {
	// Query a range of minor blocks
	resp := makeRequest("query", map[string]interface{}{
		"scope": "acc://dn.acme",
		"query": map[string]interface{}{
			"queryType": "block",
			"minorRange": map[string]interface{}{
				"start": 2460310,
				"count": 5,
			},
		},
	})
	
	if result, ok := resp["result"].(map[string]interface{}); ok {
		fmt.Printf("  Range query result type: %v\n", result["type"])
		if records, ok := result["records"].([]interface{}); ok {
			fmt.Printf("  Found %d blocks in range\n", len(records))
			for i, r := range records {
				if record, ok := r.(map[string]interface{}); ok {
					if value, ok := record["value"].(map[string]interface{}); ok {
						fmt.Printf("    Block %d: Index=%v\n", i, value["index"])
					}
				}
			}
		}
	}
}

func monitorBlocks() {
	// Try incrementing block numbers to find the actual current height
	baseHeight := uint64(2460315)
	foundLatest := false
	var actualHeight uint64
	
	fmt.Println("  Searching for actual latest block...")
	
	// Search upward from the known height
	for offset := uint64(0); offset < 1000; offset += 10 {
		testHeight := baseHeight + offset
		resp := makeRequest("query", map[string]interface{}{
			"scope": "acc://dn.acme",
			"query": map[string]interface{}{
				"queryType": "block",
				"minor": testHeight,
			},
		})
		
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if value, ok := result["value"].(map[string]interface{}); ok {
				if index, ok := value["index"].(float64); ok && index > 0 {
					actualHeight = uint64(index)
					fmt.Printf("    Found block at height %d\n", actualHeight)
				}
			}
		} else if _, ok := resp["error"].(map[string]interface{}); ok {
			// If we get an error, we've gone too far
			if offset > 0 {
				foundLatest = true
				actualHeight = baseHeight + offset - 10
				fmt.Printf("  Latest block appears to be around %d\n", actualHeight)
				break
			}
		}
		
		if offset > 100 && !foundLatest {
			fmt.Printf("  Checking up to %d...\n", testHeight)
		}
	}
	
	if !foundLatest {
		fmt.Println("  Using base height for monitoring")
		actualHeight = baseHeight
	}
	
	// Now monitor for changes
	fmt.Println("\n  Monitoring for new blocks...")
	prevHeight := actualHeight
	
	for i := 0; i < 10; i++ {
		// Check the next expected block
		checkHeight := prevHeight + 1
		resp := makeRequest("query", map[string]interface{}{
			"scope": "acc://dn.acme",
			"query": map[string]interface{}{
				"queryType": "block",
				"minor": checkHeight,
			},
		})
		
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if value, ok := result["value"].(map[string]interface{}); ok {
				if index, ok := value["index"].(float64); ok && index > 0 {
					fmt.Printf("  %s: New block found at height %d! ✅\n", 
						time.Now().Format("15:04:05"), uint64(index))
					prevHeight = uint64(index)
				}
			}
		} else {
			fmt.Printf("  %s: Waiting for block %d...\n", 
				time.Now().Format("15:04:05"), checkHeight)
		}
		
		time.Sleep(2 * time.Second)
	}
}

func makeRequest(method string, params interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}
	
	jsonData, _ := json.Marshal(payload)
	
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post("https://mainnet.accumulatenetwork.io/v3", 
		"application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("    Error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	
	return result
}