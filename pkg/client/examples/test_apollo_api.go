//go:build ignore
// +build ignore

// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func main() {
	fmt.Println("🔍 Testing Apollo/Cyclops API Endpoints")
	fmt.Println("========================================")
	
	// We know the status endpoint works, let's explore what else is available
	baseURL := "http://apollo-mainnet.accumulate.defidevs.io"
	
	// Test different ports and paths
	endpoints := []string{
		baseURL + ":16692/status",
		baseURL + ":16692/",
		baseURL + ":16695/v3",
		baseURL + ":16695/status",
		baseURL + ":26657/status",
		baseURL + ":26656/status",
		baseURL + ":8080/v3",
		baseURL + ":8080/status",
		baseURL + "/v3",
		baseURL + "/status",
	}
	
	fmt.Println("📡 Testing various Apollo endpoints:")
	for _, endpoint := range endpoints {
		fmt.Printf("\n  Testing: %s\n", endpoint)
		
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(endpoint)
		if err != nil {
			fmt.Printf("    ❌ Error: %v\n", err)
			continue
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != 200 {
			fmt.Printf("    ❌ Status: %d\n", resp.StatusCode)
			continue
		}
		
		body, _ := io.ReadAll(resp.Body)
		
		// Try to parse as JSON
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			fmt.Printf("    ❌ Not JSON: %s\n", string(body[:min(100, len(body))]))
			continue
		}
		
		fmt.Printf("    ✅ Success! Response keys: %v\n", getKeys(result))
		
		// If it has result.sync_info, show the block height
		if res, ok := result["result"].(map[string]interface{}); ok {
			if syncInfo, ok := res["sync_info"].(map[string]interface{}); ok {
				if height, ok := syncInfo["latest_block_height"].(string); ok {
					fmt.Printf("    📊 Block Height: %s\n", height)
				}
			}
		}
	}
	
	// Now let's monitor the working endpoint for changes
	fmt.Println("\n⏰ Monitoring Apollo block height (20 seconds):")
	
	var prevHeight int64
	for i := 0; i < 10; i++ {
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(baseURL + ":16692/status")
		if err == nil && resp != nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var status map[string]interface{}
			if json.Unmarshal(body, &status) == nil {
				if result, ok := status["result"].(map[string]interface{}); ok {
					if syncInfo, ok := result["sync_info"].(map[string]interface{}); ok {
						if heightStr, ok := syncInfo["latest_block_height"].(string); ok {
							var height int64
							fmt.Sscanf(heightStr, "%d", &height)
							
							if prevHeight > 0 {
								diff := height - prevHeight
								if diff > 0 {
									fmt.Printf("  %s: Block %d (+%d) ✅ CHANGING!\n", 
										time.Now().Format("15:04:05"), height, diff)
								} else {
									fmt.Printf("  %s: Block %d (unchanged)\n", 
										time.Now().Format("15:04:05"), height)
								}
							} else {
								fmt.Printf("  %s: Block %d (initial)\n", 
									time.Now().Format("15:04:05"), height)
							}
							prevHeight = height
						}
					}
				}
			}
		}
		
		time.Sleep(2 * time.Second)
	}
}

func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}