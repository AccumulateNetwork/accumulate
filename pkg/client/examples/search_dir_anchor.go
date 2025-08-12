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
	fmt.Println("🔍 Searching for DirectoryAnchor Transactions")
	fmt.Println("=============================================")
	
	// Check CYCLOPS anchor pool (where DN sends its anchors TO the BVN)
	fmt.Println("\n📊 Searching Cyclops anchor pool for DirectoryAnchors from DN:")
	
	resp := makeRequest("https://mainnet.accumulatenetwork.io/v2", "query-tx-history", map[string]interface{}{
		"url":     "acc://bvn-Cyclops.acme/anchors",
		"count":   100,
		"fromEnd": true,
	})
	
	dirAnchorsFound := 0
	latestDNHeight := uint64(0)
	
	if resp != nil {
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if items, ok := result["items"].([]interface{}); ok {
				fmt.Printf("  Checking %d transactions...\n", len(items))
				for i, item := range items {
					if tx, ok := item.(map[string]interface{}); ok {
						txType := ""
						if t, ok := tx["type"].(string); ok {
							txType = t
						}
						
						// Check if this is a directoryAnchor
						if txType == "directoryAnchor" {
							dirAnchorsFound++
							if data, ok := tx["data"].(map[string]interface{}); ok {
								if minorIdx, ok := data["minorBlockIndex"].(float64); ok {
									height := uint64(minorIdx)
									if height > latestDNHeight {
										latestDNHeight = height
									}
									if dirAnchorsFound <= 5 {
										fmt.Printf("  ✅ TX %d: DirectoryAnchor with MinorBlockIndex=%d\n", i+1, height)
									}
								}
							}
						}
					}
				}
			}
		}
	}
	
	if dirAnchorsFound > 0 {
		fmt.Printf("\n  📊 Summary:\n")
		fmt.Printf("    - Found %d DirectoryAnchor transactions\n", dirAnchorsFound)
		fmt.Printf("    - Latest DN height: %d\n", latestDNHeight)
		fmt.Printf("    - This is the REAL DN height!\n")
	} else {
		fmt.Printf("  ❌ No DirectoryAnchor transactions found\n")
	}
	
	// Compare with API
	fmt.Println("\n🔍 Comparing with Network Status API:")
	resp = makeRequest("https://mainnet.accumulatenetwork.io/v3", "network-status", map[string]interface{}{})
	
	if resp != nil {
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if dirHeight, ok := result["directoryHeight"].(float64); ok {
				fmt.Printf("  API returns DN height: %.0f\n", dirHeight)
				if uint64(dirHeight) != latestDNHeight && latestDNHeight > 0 {
					fmt.Printf("  ❌ API is wrong! Real height is %d (difference: %d)\n", 
						latestDNHeight, latestDNHeight - uint64(dirHeight))
				}
			}
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
	
	client := &http.Client{Timeout: 10 * time.Second}
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