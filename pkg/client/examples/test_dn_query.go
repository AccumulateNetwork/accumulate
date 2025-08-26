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
	fmt.Println("🔍 Testing Directory Network Queries")
	fmt.Println("=====================================")
	
	endpoints := []string{
		"https://mainnet.accumulatenetwork.io/v3",
		"https://apollo-mainnet.accumulate.defidevs.io/v3",
	}
	
	for _, endpoint := range endpoints {
		fmt.Printf("\n📡 Testing endpoint: %s\n", endpoint)
		
		// Test 1: Query DN anchors directly
		fmt.Println("\n  1. Query DN anchor pool:")
		resp := makeRequestToEndpoint(endpoint, "query", map[string]interface{}{
			"scope": "acc://dn.acme/anchors",
		})
		if result, ok := resp["result"].(map[string]interface{}); ok {
			fmt.Printf("    Result type: %v\n", result["type"])
			if records, ok := result["records"].([]interface{}); ok && len(records) > 0 {
				if record, ok := records[0].(map[string]interface{}); ok {
					if value, ok := record["value"].(map[string]interface{}); ok {
						fmt.Printf("    Account type: %v\n", value["type"])
					}
				}
			}
		}
		
		// Test 2: Query DN main chain
		fmt.Println("\n  2. Query DN main chain:")
		resp = makeRequestToEndpoint(endpoint, "query-chain", map[string]interface{}{
			"scope": "acc://dn.acme/anchors",
			"query": map[string]interface{}{
				"name": "main",
				"range": map[string]interface{}{
					"fromEnd": true,
					"count": 5,
				},
			},
		})
		
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if records, ok := result["records"].([]interface{}); ok {
				fmt.Printf("    Found %d chain entries\n", len(records))
				
				// Look for DirectoryAnchor transactions
				for i, r := range records {
					if record, ok := r.(map[string]interface{}); ok {
						if value, ok := record["value"].(map[string]interface{}); ok {
							// Check if this is a transaction
							if txn, ok := value["transaction"].(map[string]interface{}); ok {
								if body, ok := txn["body"].(map[string]interface{}); ok {
									bodyType := body["type"]
									fmt.Printf("    Entry %d: Transaction type=%v\n", i, bodyType)
									
									// If it's a DirectoryAnchor, get the minor block index
									if bodyType == "directoryAnchor" {
										if minorIndex, ok := body["minorBlockIndex"].(float64); ok {
											fmt.Printf("    🎯 Found DirectoryAnchor with MinorBlockIndex: %.0f\n", minorIndex)
										}
									}
								}
							} else {
								fmt.Printf("    Entry %d: Type=%v\n", i, value["type"])
							}
						}
					}
				}
			}
		}
		
		// Test 3: Query specific chain entry
		fmt.Println("\n  3. Query chain state:")
		resp = makeRequestToEndpoint(endpoint, "query-chain", map[string]interface{}{
			"scope": "acc://dn.acme/anchors",
			"query": map[string]interface{}{
				"name": "main",
				"state": true,
			},
		})
		
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if value, ok := result["value"].(map[string]interface{}); ok {
				fmt.Printf("    Chain state - Count: %v, Type: %v\n", value["count"], value["type"])
			}
		}
		
		// Test 4: Monitor for changes
		fmt.Println("\n  4. Monitoring chain for changes (10 seconds):")
		var prevMinorIndex float64
		
		for i := 0; i < 5; i++ {
			resp = makeRequestToEndpoint(endpoint, "query-chain", map[string]interface{}{
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
					// Look for the latest DirectoryAnchor
					for _, r := range records {
						if record, ok := r.(map[string]interface{}); ok {
							if value, ok := record["value"].(map[string]interface{}); ok {
								if txn, ok := value["transaction"].(map[string]interface{}); ok {
									if body, ok := txn["body"].(map[string]interface{}); ok {
										if body["type"] == "directoryAnchor" {
											if minorIndex, ok := body["minorBlockIndex"].(float64); ok {
												if prevMinorIndex > 0 {
													diff := minorIndex - prevMinorIndex
													if diff > 0 {
														fmt.Printf("    %s: DN Height %.0f (+%.0f) ✅ CHANGING!\n", 
															time.Now().Format("15:04:05"), minorIndex, diff)
													} else {
														fmt.Printf("    %s: DN Height %.0f (unchanged)\n", 
															time.Now().Format("15:04:05"), minorIndex)
													}
												} else {
													fmt.Printf("    %s: DN Height %.0f (initial)\n", 
														time.Now().Format("15:04:05"), minorIndex)
												}
												prevMinorIndex = minorIndex
												break // Found the latest, stop looking
											}
										}
									}
								}
							}
						}
					}
				}
			}
			
			time.Sleep(2 * time.Second)
		}
	}
}

func makeRequestToEndpoint(endpoint, method string, params interface{}) map[string]interface{} {
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
		fmt.Printf("    ❌ Error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	
	if errorData, ok := result["error"].(map[string]interface{}); ok {
		fmt.Printf("    ❌ API Error: %v\n", errorData["message"])
		return nil
	}
	
	return result
}