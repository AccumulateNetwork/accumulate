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
	fmt.Println("🔍 Testing Minor Block Queries for DN Height")
	fmt.Println("=============================================")
	
	// Test 1: Query minor blocks
	fmt.Println("\n📊 Test 1: Query minor blocks")
	testMinorBlocks()
	
	// Test 2: Query with block query type
	fmt.Println("\n📊 Test 2: Query blocks with block query type")
	testBlockQuery()
	
	// Test 3: Try query-directory-index
	fmt.Println("\n📊 Test 3: Query directory index")
	testDirectoryQuery()
	
	// Test 4: Monitor minor blocks over time
	fmt.Println("\n⏰ Test 4: Monitor minor blocks (10 seconds)")
	monitorMinorBlocks()
}

func testMinorBlocks() {
	// Try to query minor blocks
	resp := makeRequest("query", map[string]interface{}{
		"scope": "acc://dn.acme",
		"query": map[string]interface{}{
			"queryType": "block",
			"minor": map[string]interface{}{
				"start": -10,  // Last 10 blocks
				"count": 10,
			},
		},
	})
	
	if result, ok := resp["result"].(map[string]interface{}); ok {
		fmt.Printf("  Result type: %v\n", result["type"])
		if records, ok := result["records"].([]interface{}); ok {
			fmt.Printf("  Found %d records\n", len(records))
			for i, r := range records {
				if i >= 3 { break }
				if record, ok := r.(map[string]interface{}); ok {
					fmt.Printf("    Record %d: %v\n", i, record["type"])
					if value, ok := record["value"].(map[string]interface{}); ok {
						if index, ok := value["index"].(float64); ok {
							fmt.Printf("      Minor Block Index: %.0f\n", index)
						}
					}
				}
			}
		}
	}
}

func testBlockQuery() {
	// Query with explicit block parameters
	resp := makeRequest("query", map[string]interface{}{
		"scope": "Directory",  // Try Directory as scope
		"query": map[string]interface{}{
			"queryType": "block",
			"includeRemote": false,
		},
	})
	
	if result, ok := resp["result"].(map[string]interface{}); ok {
		fmt.Printf("  Result: %v\n", result)
	}
}

func testDirectoryQuery() {
	// Try query-directory method if it exists
	resp := makeRequest("query-directory", map[string]interface{}{})
	
	if result, ok := resp["result"].(map[string]interface{}); ok {
		fmt.Printf("  Directory result: %v\n", result)
	} else if errorData, ok := resp["error"].(map[string]interface{}); ok {
		fmt.Printf("  Error: %v\n", errorData["message"])
	}
	
	// Also try query with directory scope
	resp = makeRequest("query", map[string]interface{}{
		"scope": "acc://Directory",
	})
	
	if result, ok := resp["result"].(map[string]interface{}); ok {
		if records, ok := result["records"].([]interface{}); ok && len(records) > 0 {
			if record, ok := records[0].(map[string]interface{}); ok {
				fmt.Printf("  Directory query result: %v\n", record["type"])
			}
		}
	}
}

func monitorMinorBlocks() {
	var prevIndex float64
	
	for i := 0; i < 5; i++ {
		// Try to get the latest minor block
		resp := makeRequest("query", map[string]interface{}{
			"scope": "acc://dn.acme",
			"query": map[string]interface{}{
				"queryType": "block",
				"minor": map[string]interface{}{
					"start": -1,  // Last block
					"count": 1,
				},
			},
		})
		
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if records, ok := result["records"].([]interface{}); ok && len(records) > 0 {
				if record, ok := records[0].(map[string]interface{}); ok {
					if value, ok := record["value"].(map[string]interface{}); ok {
						if index, ok := value["index"].(float64); ok {
							if prevIndex > 0 {
								diff := index - prevIndex
								if diff > 0 {
									fmt.Printf("  %s: Minor Block %.0f (+%.0f) ✅ CHANGING!\n", 
										time.Now().Format("15:04:05"), index, diff)
								} else {
									fmt.Printf("  %s: Minor Block %.0f (no change)\n", 
										time.Now().Format("15:04:05"), index)
								}
							} else {
								fmt.Printf("  %s: Minor Block %.0f (initial)\n", 
									time.Now().Format("15:04:05"), index)
							}
							prevIndex = index
						}
					}
				}
			}
		}
		
		// Also check network-status again
		resp = makeRequest("network-status", map[string]interface{}{})
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if dirHeight, ok := result["directoryHeight"].(float64); ok {
				fmt.Printf("         (network-status still shows: %.0f)\n", dirHeight)
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
	
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post("https://mainnet.accumulatenetwork.io/v3", 
		"application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	
	if errorData, ok := result["error"].(map[string]interface{}); ok && method != "query-directory" {
		fmt.Printf("  API Error: %v\n", errorData["message"])
	}
	
	return result
}