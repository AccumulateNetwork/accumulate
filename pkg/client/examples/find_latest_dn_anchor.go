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
	fmt.Println("🔍 Finding Latest DirectoryAnchor Transaction")
	fmt.Println("==============================================")
	
	// Check both anchor pools
	endpoints := []struct {
		name string
		url  string
	}{
		{"DN anchors", "acc://dn.acme/anchors"},
		{"Cyclops anchors", "acc://bvn-Cyclops.acme/anchors"},
	}
	
	for _, endpoint := range endpoints {
		fmt.Printf("\n📊 Checking %s (%s):\n", endpoint.name, endpoint.url)
		
		// Search through transaction history to find DirectoryAnchors
		foundCount := 0
		latestHeight := uint64(0)
		
		// Try to get more transactions
		for start := 0; start < 500 && foundCount < 10; start += 50 {
			resp := makeRequest("https://mainnet.accumulatenetwork.io/v2", "query-tx-history", map[string]interface{}{
				"url":   endpoint.url,
				"start": start,
				"count": 50,
			})
			
			if resp == nil {
				break
			}
			
			if result, ok := resp["result"].(map[string]interface{}); ok {
				if items, ok := result["items"].([]interface{}); ok {
					for _, item := range items {
						if tx, ok := item.(map[string]interface{}); ok {
							if data, ok := tx["data"].(map[string]interface{}); ok {
								if body, ok := data["body"].(map[string]interface{}); ok {
									if body["type"] == "directoryAnchor" {
										foundCount++
										if minorIdx, ok := body["minorBlockIndex"].(float64); ok {
											height := uint64(minorIdx)
											if height > latestHeight {
												latestHeight = height
											}
											if foundCount <= 5 {
												// Show details of first few
												fmt.Printf("  ✅ Found DirectoryAnchor: MinorBlockIndex=%d", height)
												if header, ok := data["header"].(map[string]interface{}); ok {
													if principal, ok := header["principal"].(string); ok {
														fmt.Printf(" (from %s)", principal)
													}
												}
												fmt.Println()
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
		
		if foundCount > 0 {
			fmt.Printf("  📊 Found %d DirectoryAnchor transactions\n", foundCount)
			fmt.Printf("  📍 Latest DN height seen: %d\n", latestHeight)
		} else {
			fmt.Printf("  ❌ No DirectoryAnchor transactions found\n")
		}
	}
	
	// Now test the network status API to see what it returns
	fmt.Println("\n🔍 Testing Network Status API:")
	resp := makeRequest("https://mainnet.accumulatenetwork.io/v3", "network-status", map[string]interface{}{})
	
	if resp != nil {
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if dirHeight, ok := result["directoryHeight"].(float64); ok {
				fmt.Printf("  API returns DN height: %.0f\n", dirHeight)
				if dirHeight == 2460315 {
					fmt.Println("  ❌ This is the CACHED/STATIC value!")
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