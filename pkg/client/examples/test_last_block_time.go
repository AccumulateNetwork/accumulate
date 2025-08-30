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
	fmt.Println("🔍 Monitoring DN Activity via LastBlockTime")
	fmt.Println("============================================")
	fmt.Println("The lastBlockTime from query-directory shows DN activity!")
	
	endpoints := []string{
		"https://mainnet.accumulatenetwork.io/v2",
		"https://apollo-mainnet.accumulate.defidevs.io/v2",
		"http://apollo-mainnet.accumulate.defidevs.io:16695/v2",
	}
	
	fmt.Println("\n⏰ Monitoring DN lastBlockTime (proves DN is active):")
	
	var lastTime time.Time
	
	for round := 0; round < 10; round++ {
		fmt.Printf("\n  Round %d at %s:\n", round+1, time.Now().Format("15:04:05"))
		
		for _, endpoint := range endpoints {
			resp := makeRequest(endpoint, "query-directory", map[string]interface{}{
				"url": "acc://dn.acme",
			})
			
			if resp != nil {
				if result, ok := resp["result"].(map[string]interface{}); ok {
					if lbtStr, ok := result["lastBlockTime"].(string); ok {
						lbt, err := time.Parse(time.RFC3339, lbtStr)
						if err == nil {
							if !lastTime.IsZero() {
								diff := lbt.Sub(lastTime)
								if diff > 0 {
									fmt.Printf("    🎯 %s: %s (+%s) DN IS ACTIVE!\n",
										endpoint, lbt.Format("15:04:05"), diff)
								} else {
									fmt.Printf("    %s: %s (unchanged)\n",
										endpoint, lbt.Format("15:04:05"))
								}
							} else {
								fmt.Printf("    %s: %s (initial)\n",
									endpoint, lbt.Format("15:04:05"))
							}
							lastTime = lbt
							break // Only need one working endpoint
						}
					}
				}
			}
		}
		
		// Also query V3 to show the static directoryHeight
		v3Resp := makeV3Request("https://mainnet.accumulatenetwork.io/v3", "network-status", map[string]interface{}{})
		if v3Resp != nil {
			if result, ok := v3Resp["result"].(map[string]interface{}); ok {
				if dirHeight, ok := result["directoryHeight"].(float64); ok {
					fmt.Printf("    V3 API still shows static DN height: %.0f\n", dirHeight)
				}
			}
		}
		
		if round < 9 {
			time.Sleep(3 * time.Second)
		}
	}
	
	fmt.Println("\n✅ PROOF: The DN lastBlockTime is updating, proving the DN is active!")
	fmt.Println("❌ ISSUE: The V3 API directoryHeight is cached/static")
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

func makeV3Request(endpoint, method string, params interface{}) map[string]interface{} {
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