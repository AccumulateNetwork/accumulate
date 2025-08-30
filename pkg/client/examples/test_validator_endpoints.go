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
	fmt.Println("🔍 Testing for Live DN Endpoints")
	fmt.Println("==================================")
	fmt.Println("Searching for endpoints that show changing DN height...")
	
	// Try different possible mainnet validator/node endpoints
	endpoints := []struct {
		name string
		urls []string
	}{
		{
			name: "Directory Network Direct",
			urls: []string{
				"http://directory.accumulatenetwork.io/v3",
				"https://directory.accumulatenetwork.io/v3",
				"http://dn.accumulatenetwork.io/v3",
				"https://dn.accumulatenetwork.io/v3",
				"http://mainnet-dn.accumulatenetwork.io/v3",
				"https://mainnet-dn.accumulatenetwork.io/v3",
			},
		},
		{
			name: "Validator Nodes",
			urls: []string{
				"http://validator.accumulatenetwork.io/v3",
				"https://validator.accumulatenetwork.io/v3",
				"http://mainnet-validator.accumulatenetwork.io/v3",
				"https://mainnet-validator.accumulatenetwork.io/v3",
				"http://node.accumulatenetwork.io/v3",
				"https://node.accumulatenetwork.io/v3",
			},
		},
		{
			name: "Alternative Ports on Apollo",
			urls: []string{
				"http://apollo-mainnet.accumulate.defidevs.io:16695/v3",
				"http://apollo-mainnet.accumulate.defidevs.io:16696/v3",
				"http://apollo-mainnet.accumulate.defidevs.io:8080/v3",
				"http://apollo-mainnet.accumulate.defidevs.io:8081/v3",
				"http://apollo-mainnet.accumulate.defidevs.io:9090/v3",
			},
		},
		{
			name: "DefiDevs Alternative Endpoints",
			urls: []string{
				"http://directory.accumulate.defidevs.io/v3",
				"https://directory.accumulate.defidevs.io/v3",
				"http://dn.accumulate.defidevs.io/v3",
				"https://dn.accumulate.defidevs.io/v3",
				"http://mainnet-dn.accumulate.defidevs.io/v3",
				"https://mainnet-dn.accumulate.defidevs.io/v3",
			},
		},
	}
	
	var workingEndpoints []string
	
	for _, group := range endpoints {
		fmt.Printf("\n📡 Testing %s:\n", group.name)
		for _, endpoint := range group.urls {
			fmt.Printf("  Testing: %s\n", endpoint)
			
			resp := makeRequest(endpoint, "network-status", map[string]interface{}{})
			if resp != nil {
				if result, ok := resp["result"].(map[string]interface{}); ok {
					if dirHeight, ok := result["directoryHeight"].(float64); ok {
						fmt.Printf("    ✅ Directory Height: %.0f\n", dirHeight)
						workingEndpoints = append(workingEndpoints, endpoint)
					}
				}
			}
		}
	}
	
	// Now test the working endpoints to see if any show changing DN height
	if len(workingEndpoints) > 0 {
		fmt.Println("\n⏰ Monitoring working endpoints for DN height changes:")
		
		// Track heights for each endpoint
		heights := make(map[string]float64)
		
		for round := 0; round < 5; round++ {
			fmt.Printf("\n  Round %d at %s:\n", round+1, time.Now().Format("15:04:05"))
			
			for _, endpoint := range workingEndpoints {
				resp := makeRequest(endpoint, "network-status", map[string]interface{}{})
				if resp != nil {
					if result, ok := resp["result"].(map[string]interface{}); ok {
						if dirHeight, ok := result["directoryHeight"].(float64); ok {
							prevHeight := heights[endpoint]
							heights[endpoint] = dirHeight
							
							if prevHeight > 0 && dirHeight != prevHeight {
								fmt.Printf("    🎯 %s: %.0f → %.0f (+%.0f) CHANGING!\n", 
									endpoint, prevHeight, dirHeight, dirHeight-prevHeight)
							} else if prevHeight > 0 {
								fmt.Printf("    %s: %.0f (unchanged)\n", endpoint, dirHeight)
							} else {
								fmt.Printf("    %s: %.0f (initial)\n", endpoint, dirHeight)
							}
						}
					}
				}
			}
			
			if round < 4 {
				time.Sleep(3 * time.Second)
			}
		}
	}
	
	// Also try to find if there's a special partition parameter we need
	fmt.Println("\n🔍 Testing partition parameters:")
	testEndpoint := "https://mainnet.accumulatenetwork.io/v3"
	partitions := []string{"Directory", "directory", "DN", "dn", "MainNet", "mainnet"}
	
	for _, partition := range partitions {
		fmt.Printf("\n  Testing partition '%s':\n", partition)
		resp := makeRequest(testEndpoint, "network-status", map[string]interface{}{
			"partition": partition,
		})
		
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				if dirHeight, ok := result["directoryHeight"].(float64); ok {
					fmt.Printf("    Directory Height: %.0f\n", dirHeight)
				}
			} else if errorData, ok := resp["error"].(map[string]interface{}); ok {
				fmt.Printf("    Error: %v\n", errorData["message"])
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
	
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("    ❌ Error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil
	}
	
	if errorData, ok := result["error"].(map[string]interface{}); ok {
		// Only print for non-404 errors
		if msg, ok := errorData["message"].(string); ok && msg != "Method not found" {
			fmt.Printf("    ❌ API Error: %v\n", msg)
		}
		return nil
	}
	
	return result
}