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
	fmt.Println("🔍 Testing Current Mainnet Configuration")
	fmt.Println("=========================================")
	fmt.Println("(Mainnet has been reduced to one BVN)")
	
	// Test different possible endpoints
	endpoints := []string{
		"https://mainnet.accumulatenetwork.io/v3",
		"http://mainnet.accumulatenetwork.io/v3",
		"https://apollo-mainnet.accumulate.defidevs.io/v3",
		"http://apollo-mainnet.accumulate.defidevs.io/v3",
		"http://cyclops-mainnet.accumulate.defidevs.io/v3",
		"https://cyclops-mainnet.accumulate.defidevs.io/v3",
		"http://mainnet.accumulate.defidevs.io/v3",
		"https://mainnet.accumulate.defidevs.io/v3",
	}
	
	fmt.Println("\n📡 Testing various endpoints for network-status:")
	for _, endpoint := range endpoints {
		fmt.Printf("\n  Testing: %s\n", endpoint)
		resp := makeRequestToEndpoint(endpoint, "network-status", map[string]interface{}{})
		
		if resp == nil {
			continue
		}
		
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if dirHeight, ok := result["directoryHeight"].(float64); ok {
				fmt.Printf("    ✅ Directory Height: %.0f\n", dirHeight)
				
				// Also check network configuration
				if network, ok := result["network"].(map[string]interface{}); ok {
					if partitions, ok := network["partitions"].([]interface{}); ok {
						fmt.Printf("    Partitions: %d\n", len(partitions))
						for _, p := range partitions {
							if part, ok := p.(map[string]interface{}); ok {
								fmt.Printf("      - %v (%v)\n", part["id"], part["type"])
							}
						}
					}
				}
			}
		}
	}
	
	// Since there's only one BVN now, try to get its status directly
	fmt.Println("\n🔍 Testing Cyclops/Apollo endpoints (the single BVN):")
	
	// Test the status endpoint we know works
	fmt.Println("\n  Testing: http://apollo-mainnet.accumulate.defidevs.io:16692/status")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://apollo-mainnet.accumulate.defidevs.io:16692/status")
	if err == nil && resp != nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var status map[string]interface{}
		if json.Unmarshal(body, &status) == nil {
			if result, ok := status["result"].(map[string]interface{}); ok {
				if syncInfo, ok := result["sync_info"].(map[string]interface{}); ok {
					if height, ok := syncInfo["latest_block_height"].(string); ok {
						fmt.Printf("    ✅ Cyclops Block Height: %s\n", height)
					}
				}
			}
		}
	}
	
	// Monitor for changes on working endpoints
	fmt.Println("\n⏰ Monitoring for DN height changes (20 seconds):")
	
	// Find a working endpoint first
	var workingEndpoint string
	for _, endpoint := range endpoints {
		resp := makeRequestToEndpoint(endpoint, "network-status", map[string]interface{}{})
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				if _, ok := result["directoryHeight"].(float64); ok {
					workingEndpoint = endpoint
					fmt.Printf("  Using endpoint: %s\n", workingEndpoint)
					break
				}
			}
		}
	}
	
	if workingEndpoint != "" {
		var prevHeight float64
		for i := 0; i < 10; i++ {
			resp := makeRequestToEndpoint(workingEndpoint, "network-status", map[string]interface{}{})
			if result, ok := resp["result"].(map[string]interface{}); ok {
				if dirHeight, ok := result["directoryHeight"].(float64); ok {
					if prevHeight > 0 {
						diff := dirHeight - prevHeight
						if diff > 0 {
							fmt.Printf("  %s: DN Height %.0f (+%.0f) ✅ CHANGING!\n", 
								time.Now().Format("15:04:05"), dirHeight, diff)
						} else {
							fmt.Printf("  %s: DN Height %.0f (unchanged)\n", 
								time.Now().Format("15:04:05"), dirHeight)
						}
					} else {
						fmt.Printf("  %s: DN Height %.0f (initial)\n", 
							time.Now().Format("15:04:05"), dirHeight)
					}
					prevHeight = dirHeight
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
	
	client := &http.Client{Timeout: 5 * time.Second}
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