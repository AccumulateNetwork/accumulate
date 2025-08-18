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
	fmt.Println("🔍 Inspecting Anchor Pool Contents")
	fmt.Println("===================================")
	
	// Query the DN's anchor pool to see what's actually there
	endpoints := map[string]string{
		"DN anchors":      "acc://dn.acme/anchors",
		"Cyclops anchors": "acc://bvn-Cyclops.acme/anchors",
	}
	
	for name, url := range endpoints {
		fmt.Printf("\n📊 %s (%s):\n", name, url)
		
		// Query the account
		resp := makeRequest("https://mainnet.accumulatenetwork.io/v2", "query", map[string]interface{}{
			"url": url,
		})
		
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				// Get the data field
				if data, ok := result["data"].(map[string]interface{}); ok {
					fmt.Printf("  Type: %v\n", data["type"])
					if seqNum, ok := data["minorBlockSequenceNumber"].(float64); ok {
						fmt.Printf("  Minor Block Sequence: %.0f\n", seqNum)
					}
					if majorIdx, ok := data["majorBlockIndex"].(float64); ok {
						fmt.Printf("  Major Block Index: %.0f\n", majorIdx)
					}
					
					// Check the sequence info
					if sequence, ok := data["sequence"].([]interface{}); ok {
						fmt.Printf("  Sequence (%d entries):\n", len(sequence))
						for _, s := range sequence {
							if seq, ok := s.(map[string]interface{}); ok {
								fmt.Printf("    - %s: received=%.0f delivered=%.0f\n",
									seq["url"], seq["received"], seq["delivered"])
							}
						}
					}
				}
				
				// Check the main chain
				if mainChain, ok := result["mainChain"].(map[string]interface{}); ok {
					if height, ok := mainChain["height"].(float64); ok {
						fmt.Printf("  Main Chain Height: %.0f\n", height)
					}
					if count, ok := mainChain["count"].(float64); ok {
						fmt.Printf("  Main Chain Count: %.0f\n", count)
					}
				}
			}
		}
		
		// Now query for recent transactions on this anchor pool
		fmt.Printf("\n  Recent transactions:\n")
		resp = makeRequest("https://mainnet.accumulatenetwork.io/v2", "query-tx-history", map[string]interface{}{
			"url":   url,
			"count": 5,
		})
		
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				if items, ok := result["items"].([]interface{}); ok {
					for i, item := range items {
						if tx, ok := item.(map[string]interface{}); ok {
							fmt.Printf("    TX %d: type=%v\n", i+1, tx["type"])
							if data, ok := tx["data"].(map[string]interface{}); ok {
								if body, ok := data["body"].(map[string]interface{}); ok {
									bodyType := body["type"]
									fmt.Printf("      Body type: %v\n", bodyType)
									if bodyType == "directoryAnchor" {
										if minorIdx, ok := body["minorBlockIndex"].(float64); ok {
											fmt.Printf("      🎯 DirectoryAnchor MinorBlockIndex: %.0f\n", minorIdx)
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
	
	// Try querying the main chain directly
	fmt.Println("\n📊 Query DN anchors main chain:")
	resp := makeRequest("https://mainnet.accumulatenetwork.io/v2", "query-chain", map[string]interface{}{
		"url":   "acc://dn.acme/anchors",
		"name":  "main",
		"count": 5,
		"fromEnd": true,
	})
	
	if resp != nil {
		if result, ok := resp["result"].(map[string]interface{}); ok {
			fmt.Printf("  Result type: %v\n", result["type"])
			if items, ok := result["items"].([]interface{}); ok {
				fmt.Printf("  Found %d chain entries\n", len(items))
				for i, item := range items {
					if entry, ok := item.(map[string]interface{}); ok {
						fmt.Printf("    Entry %d: %v\n", i, entry)
					}
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