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
	fmt.Println("🔍 Testing DN Height Retrieval Methods")
	fmt.Println("=======================================")
	
	// Test 1: network-status endpoint
	fmt.Println("\n📊 Test 1: Using network-status endpoint")
	testNetworkStatus()
	
	// Test 2: Query DN anchor pool directly
	fmt.Println("\n⚓ Test 2: Query DN anchor pool directly")
	testAnchorPool()
	
	// Test 3: Query chains endpoint
	fmt.Println("\n🔗 Test 3: Query chains endpoint for DN")
	testChains()
	
	// Test 4: Try query-chain for anchor pool
	fmt.Println("\n📈 Test 4: Query chain entries")
	testChainEntries()
	
	// Test 5: Monitor changes over time
	fmt.Println("\n⏰ Test 5: Monitor DN height changes (10 seconds)")
	monitorChanges()
}

func testNetworkStatus() {
	resp := makeRequest("network-status", map[string]interface{}{})
	if result, ok := resp["result"].(map[string]interface{}); ok {
		if dirHeight, ok := result["directoryHeight"].(float64); ok {
			fmt.Printf("  Directory Height: %.0f\n", dirHeight)
		}
		if majorHeight, ok := result["majorBlockHeight"].(float64); ok {
			fmt.Printf("  Major Block Height: %.0f\n", majorHeight)
		}
	}
}

func testAnchorPool() {
	// Query the DN anchor pool account
	resp := makeRequest("query", map[string]interface{}{
		"scope": "acc://dn.acme/anchors",
	})
	
	if result, ok := resp["result"].(map[string]interface{}); ok {
		fmt.Printf("  Anchor Pool Response: %v\n", result["type"])
		if records, ok := result["records"].([]interface{}); ok && len(records) > 0 {
			if record, ok := records[0].(map[string]interface{}); ok {
				if value, ok := record["value"].(map[string]interface{}); ok {
					fmt.Printf("  Account Type: %v\n", value["type"])
				}
			}
		}
	}
}

func testChains() {
	// Query chains for the DN
	resp := makeRequest("query", map[string]interface{}{
		"scope": "acc://dn.acme/anchors",
		"query": map[string]interface{}{
			"queryType": "chain",
			"name": "main",
		},
	})
	
	if result, ok := resp["result"].(map[string]interface{}); ok {
		if records, ok := result["records"].([]interface{}); ok {
			fmt.Printf("  Found %d chain records\n", len(records))
			// Look for the latest entries
			for i, r := range records {
				if i >= 5 { break } // Only show first 5
				if record, ok := r.(map[string]interface{}); ok {
					if value, ok := record["value"].(map[string]interface{}); ok {
						fmt.Printf("    Entry %d: Type=%v\n", i, value["type"])
					}
				}
			}
		}
	}
}

func testChainEntries() {
	// Try to get chain entries with more detail
	resp := makeRequest("query-chain", map[string]interface{}{
		"scope": "acc://dn.acme/anchors",
		"query": map[string]interface{}{
			"name": "main",
			"range": map[string]interface{}{
				"fromEnd": true,
				"count": 10,
			},
		},
	})
	
	if result, ok := resp["result"].(map[string]interface{}); ok {
		if records, ok := result["records"].([]interface{}); ok {
			fmt.Printf("  Found %d main chain entries\n", len(records))
			
			// Look for DirectoryAnchor transactions
			for _, r := range records {
				if record, ok := r.(map[string]interface{}); ok {
					if value, ok := record["value"].(map[string]interface{}); ok {
						if txn, ok := value["transaction"].(map[string]interface{}); ok {
							if body, ok := txn["body"].(map[string]interface{}); ok {
								if bodyType, ok := body["type"].(string); ok && bodyType == "directoryAnchor" {
									if minorIndex, ok := body["minorBlockIndex"].(float64); ok {
										fmt.Printf("  🎯 Found DirectoryAnchor with MinorBlockIndex: %.0f\n", minorIndex)
										return
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

func monitorChanges() {
	var prevHeight float64
	
	for i := 0; i < 5; i++ {
		resp := makeRequest("network-status", map[string]interface{}{})
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if dirHeight, ok := result["directoryHeight"].(float64); ok {
				if prevHeight > 0 {
					diff := dirHeight - prevHeight
					if diff > 0 {
						fmt.Printf("  %s: Height %.0f (+%.0f) ✅ CHANGING!\n", 
							time.Now().Format("15:04:05"), dirHeight, diff)
					} else {
						fmt.Printf("  %s: Height %.0f (no change)\n", 
							time.Now().Format("15:04:05"), dirHeight)
					}
				} else {
					fmt.Printf("  %s: Height %.0f (initial)\n", 
						time.Now().Format("15:04:05"), dirHeight)
				}
				prevHeight = dirHeight
			}
		}
		
		if i < 4 {
			time.Sleep(2 * time.Second)
		}
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
	
	resp, err := http.Post("https://mainnet.accumulatenetwork.io/v3", 
		"application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	
	if errorData, ok := result["error"].(map[string]interface{}); ok {
		fmt.Printf("  API Error: %v\n", errorData["message"])
	}
	
	return result
}