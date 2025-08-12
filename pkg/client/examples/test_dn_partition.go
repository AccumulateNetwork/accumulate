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
	fmt.Println("🔍 Testing DN Height with Different Partitions")
	fmt.Println("===============================================")
	
	// Test different partition parameters
	partitions := []string{"", "Directory", "BVN0", "BVN1", "BVN2"}
	
	for _, partition := range partitions {
		fmt.Printf("\n📊 Testing partition: '%s'\n", partition)
		
		params := map[string]interface{}{}
		if partition != "" {
			params["partition"] = partition
		}
		
		resp := makeRequest("network-status", params)
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if dirHeight, ok := result["directoryHeight"].(float64); ok {
				fmt.Printf("  Directory Height: %.0f\n", dirHeight)
			} else {
				fmt.Printf("  Directory Height: not found\n")
			}
			
			if majorHeight, ok := result["majorBlockHeight"].(float64); ok {
				fmt.Printf("  Major Block Height: %.0f\n", majorHeight)
			}
		} else if errorData, ok := resp["error"].(map[string]interface{}); ok {
			fmt.Printf("  Error: %v\n", errorData["message"])
		}
	}
	
	// Now monitor the Directory partition specifically
	fmt.Println("\n⏰ Monitoring Directory partition (10 seconds)")
	var prevHeight float64
	
	for i := 0; i < 5; i++ {
		resp := makeRequest("network-status", map[string]interface{}{
			"partition": "Directory",
		})
		
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
	
	// Try different endpoints
	fmt.Println("\n🌐 Testing different mainnet endpoints")
	endpoints := []string{
		"https://mainnet.accumulatenetwork.io/v3",
		"https://mainnet-bvn0.accumulatenetwork.io/v3",
		"https://mainnet-bvn1.accumulatenetwork.io/v3",
		"https://mainnet-bvn2.accumulatenetwork.io/v3",
	}
	
	for _, endpoint := range endpoints {
		fmt.Printf("\n  Endpoint: %s\n", endpoint)
		resp := makeRequestToEndpoint(endpoint, "network-status", map[string]interface{}{})
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if dirHeight, ok := result["directoryHeight"].(float64); ok {
				fmt.Printf("    Directory Height: %.0f\n", dirHeight)
			}
		}
	}
}

func makeRequest(method string, params interface{}) map[string]interface{} {
	return makeRequestToEndpoint("https://mainnet.accumulatenetwork.io/v3", method, params)
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
		fmt.Printf("    Error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	
	return result
}