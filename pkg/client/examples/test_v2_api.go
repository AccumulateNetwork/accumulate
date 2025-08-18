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
	fmt.Println("🔍 Testing V2 API for Directory Anchor Height")
	fmt.Println("==============================================")
	
	endpoints := []string{
		"https://mainnet.accumulatenetwork.io/v2",
		"https://apollo-mainnet.accumulate.defidevs.io/v2",
		"http://apollo-mainnet.accumulate.defidevs.io:16695/v2",
	}
	
	// Test V2 status endpoint which has LastDirectoryAnchorHeight
	for _, endpoint := range endpoints {
		fmt.Printf("\n📡 Testing V2 endpoint: %s\n", endpoint)
		
		// Test status method
		resp := makeRequest(endpoint, "status", map[string]interface{}{})
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				fmt.Printf("  Status response:\n")
				if ldah, ok := result["lastDirectoryAnchorHeight"].(float64); ok {
					fmt.Printf("    LastDirectoryAnchorHeight: %.0f\n", ldah)
				}
				if dnHeight, ok := result["dnHeight"].(float64); ok {
					fmt.Printf("    DN Height: %.0f\n", dnHeight)
				}
				if bvnHeight, ok := result["bvnHeight"].(float64); ok {
					fmt.Printf("    BVN Height: %.0f\n", bvnHeight)
				}
			}
		}
		
		// Test faucet (which is a BVN-specific endpoint)
		fmt.Println("\n  Testing faucet endpoint (BVN-specific):")
		resp = makeRequest(endpoint, "faucet", map[string]interface{}{
			"url": "acc://test.acme",
		})
		if resp != nil {
			if _, ok := resp["error"].(map[string]interface{}); ok {
				fmt.Println("    Faucet not available (expected)")
			}
		}
		
		// Test query-directory for DN
		fmt.Println("\n  Testing query-directory:")
		resp = makeRequest(endpoint, "query-directory", map[string]interface{}{
			"url": "acc://dn.acme",
		})
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				fmt.Printf("    Directory query returned: %v\n", result)
			}
		}
	}
	
	// Monitor V2 status for changes
	fmt.Println("\n⏰ Monitoring V2 status for changes (10 seconds):")
	
	var prevHeight float64
	workingEndpoint := ""
	
	// Find a working V2 endpoint
	for _, endpoint := range endpoints {
		resp := makeRequest(endpoint, "status", map[string]interface{}{})
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				if _, ok := result["lastDirectoryAnchorHeight"]; ok {
					workingEndpoint = endpoint
					fmt.Printf("  Using endpoint: %s\n", workingEndpoint)
					break
				}
			}
		}
	}
	
	if workingEndpoint != "" {
		for i := 0; i < 5; i++ {
			resp := makeRequest(workingEndpoint, "status", map[string]interface{}{})
			if resp != nil {
				if result, ok := resp["result"].(map[string]interface{}); ok {
					if ldah, ok := result["lastDirectoryAnchorHeight"].(float64); ok {
						if prevHeight > 0 && ldah != prevHeight {
							fmt.Printf("  %s: LDAH %.0f → %.0f (+%.0f) ✅ CHANGING!\n",
								time.Now().Format("15:04:05"), prevHeight, ldah, ldah-prevHeight)
						} else if prevHeight > 0 {
							fmt.Printf("  %s: LDAH %.0f (unchanged)\n",
								time.Now().Format("15:04:05"), ldah)
						} else {
							fmt.Printf("  %s: LDAH %.0f (initial)\n",
								time.Now().Format("15:04:05"), ldah)
						}
						prevHeight = ldah
					}
				}
			}
			
			if i < 4 {
				time.Sleep(2 * time.Second)
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
		fmt.Printf("    ❌ Error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	
	if errorData, ok := result["error"].(map[string]interface{}); ok {
		// Only show non-404 errors
		if msg, ok := errorData["message"].(string); ok && msg != "Method not found" {
			fmt.Printf("    ❌ API Error: %v\n", msg)
		}
	}
	
	return result
}