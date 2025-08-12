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
	fmt.Println("🔍 Finding Real DN Height from Available Data")
	fmt.Println("==============================================")
	
	// We know:
	// 1. DN is active (lastBlockTime updates every ~3 seconds)
	// 2. V3 API shows cached height of 2460315
	// 3. DN produces blocks at ~0.3-0.5 blocks/second
	
	// Let's query the DN anchors account to see if we can get more info
	endpoints := []string{
		"https://mainnet.accumulatenetwork.io",
		"https://apollo-mainnet.accumulate.defidevs.io",
	}
	
	for _, baseEndpoint := range endpoints {
		fmt.Printf("\n📡 Testing endpoint: %s\n", baseEndpoint)
		
		// Try V2 query for DN anchors
		fmt.Println("\n  V2 Query DN anchors account:")
		v2Resp := makeRequest(baseEndpoint+"/v2", "query", map[string]interface{}{
			"url": "acc://dn.acme/anchors",
		})
		
		if v2Resp != nil {
			if result, ok := v2Resp["result"].(map[string]interface{}); ok {
				fmt.Printf("    Type: %v\n", result["type"])
				if mainChain, ok := result["mainChain"].(map[string]interface{}); ok {
					if height, ok := mainChain["height"].(float64); ok {
						fmt.Printf("    🎯 Main Chain Height: %.0f\n", height)
					}
					if count, ok := mainChain["count"].(float64); ok {
						fmt.Printf("    Main Chain Count: %.0f\n", count)
					}
				}
				if data, ok := result["data"].(map[string]interface{}); ok {
					fmt.Printf("    Data: %v\n", data)
				}
			}
		}
		
		// Try V2 query-chain for DN anchors
		fmt.Println("\n  V2 Query DN anchors main chain:")
		v2Resp = makeRequest(baseEndpoint+"/v2", "query-chain", map[string]interface{}{
			"url": "acc://dn.acme/anchors#main",
		})
		
		if v2Resp != nil {
			if result, ok := v2Resp["result"].(map[string]interface{}); ok {
				fmt.Printf("    Type: %v\n", result["type"])
				if height, ok := result["height"].(float64); ok {
					fmt.Printf("    🎯 Chain Height: %.0f\n", height)
				}
				if count, ok := result["count"].(float64); ok {
					fmt.Printf("    🎯 Chain Count: %.0f\n", count)
				}
				if mdRoot, ok := result["mdRoot"].(string); ok {
					fmt.Printf("    MD Root: %s\n", mdRoot[:16]+"...")
				}
			}
		}
		
		// Try V2 query-tx-history for DN
		fmt.Println("\n  V2 Query DN transaction history:")
		v2Resp = makeRequest(baseEndpoint+"/v2", "query-tx-history", map[string]interface{}{
			"url":   "acc://dn.acme",
			"count": 1,
		})
		
		if v2Resp != nil {
			if result, ok := v2Resp["result"].(map[string]interface{}); ok {
				if items, ok := result["items"].([]interface{}); ok && len(items) > 0 {
					fmt.Printf("    Found %d transactions\n", len(items))
					if tx, ok := items[0].(map[string]interface{}); ok {
						fmt.Printf("    Latest TX Type: %v\n", tx["type"])
					}
				}
			}
		}
		
		// Try V3 query with different parameters
		fmt.Println("\n  V3 Query with includeRemote:")
		v3Resp := makeRequest(baseEndpoint+"/v3", "query", map[string]interface{}{
			"scope": "acc://dn.acme",
			"query": map[string]interface{}{
				"includeRemote": true,
			},
		})
		
		if v3Resp != nil {
			if result, ok := v3Resp["result"].(map[string]interface{}); ok {
				if records, ok := result["records"].([]interface{}); ok && len(records) > 0 {
					if record, ok := records[0].(map[string]interface{}); ok {
						if value, ok := record["value"].(map[string]interface{}); ok {
							fmt.Printf("    Record type: %v\n", value["type"])
						}
					}
				}
			}
		}
	}
	
	// Monitor chain height changes
	fmt.Println("\n⏰ Monitoring for chain height changes:")
	
	var prevHeight float64
	for i := 0; i < 5; i++ {
		resp := makeRequest("https://mainnet.accumulatenetwork.io/v2", "query-chain", map[string]interface{}{
			"url": "acc://dn.acme/anchors#main",
		})
		
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				if height, ok := result["height"].(float64); ok {
					if prevHeight > 0 && height != prevHeight {
						fmt.Printf("  %s: Chain Height %.0f → %.0f (+%.0f) ✅ CHANGING!\n",
							time.Now().Format("15:04:05"), prevHeight, height, height-prevHeight)
					} else if prevHeight > 0 {
						fmt.Printf("  %s: Chain Height %.0f (unchanged)\n",
							time.Now().Format("15:04:05"), height)
					} else {
						fmt.Printf("  %s: Chain Height %.0f (initial)\n",
							time.Now().Format("15:04:05"), height)
					}
					prevHeight = height
				}
			}
		}
		
		if i < 4 {
			time.Sleep(3 * time.Second)
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