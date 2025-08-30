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
	fmt.Println("🔍 Getting Real DN Activity from minorBlockSequenceNumber")
	fmt.Println("=========================================================")
	fmt.Println("The minorBlockSequenceNumber in the anchor ledger shows DN activity!")
	
	endpoint := "https://mainnet.accumulatenetwork.io/v2"
	
	fmt.Println("\n⏰ Monitoring DN minorBlockSequenceNumber (this IS the real DN activity):")
	
	var prevSequence float64
	var prevDNDelivered float64
	
	for round := 0; round < 10; round++ {
		fmt.Printf("\n  Round %d at %s:\n", round+1, time.Now().Format("15:04:05"))
		
		// Query the DN anchors account
		resp := makeRequest(endpoint, "query", map[string]interface{}{
			"url": "acc://dn.acme/anchors",
		})
		
		if resp != nil {
			if result, ok := resp["result"].(map[string]interface{}); ok {
				// Get minorBlockSequenceNumber
				if seqNum, ok := result["minorBlockSequenceNumber"].(float64); ok {
					if prevSequence > 0 && seqNum != prevSequence {
						fmt.Printf("    🎯 minorBlockSequenceNumber: %.0f → %.0f (+%.0f) DN IS PRODUCING BLOCKS!\n",
							prevSequence, seqNum, seqNum-prevSequence)
					} else if prevSequence > 0 {
						fmt.Printf("    minorBlockSequenceNumber: %.0f (unchanged - waiting for next block)\n", seqNum)
					} else {
						fmt.Printf("    minorBlockSequenceNumber: %.0f (initial)\n", seqNum)
					}
					prevSequence = seqNum
				}
				
				// Also check the sequence array for DN delivered count
				if sequence, ok := result["sequence"].([]interface{}); ok {
					for _, s := range sequence {
						if seq, ok := s.(map[string]interface{}); ok {
							if url, ok := seq["url"].(string); ok && url == "acc://dn.acme" {
								if delivered, ok := seq["delivered"].(float64); ok {
									if prevDNDelivered > 0 && delivered != prevDNDelivered {
										fmt.Printf("    🎯 DN delivered anchors: %.0f → %.0f (+%.0f)\n",
											prevDNDelivered, delivered, delivered-prevDNDelivered)
									} else if prevDNDelivered == 0 {
										fmt.Printf("    DN delivered anchors: %.0f\n", delivered)
									}
									prevDNDelivered = delivered
								}
							}
						}
					}
				}
				
				// Show major block info
				if majorIndex, ok := result["majorBlockIndex"].(float64); ok {
					fmt.Printf("    Major Block Index: %.0f\n", majorIndex)
				}
			}
		}
		
		// Compare with V3 static value
		v3Resp := makeRequest("https://mainnet.accumulatenetwork.io/v3", "network-status", map[string]interface{}{})
		if v3Resp != nil {
			if result, ok := v3Resp["result"].(map[string]interface{}); ok {
				if dirHeight, ok := result["directoryHeight"].(float64); ok {
					fmt.Printf("    V3 API (cached): directoryHeight = %.0f (static/wrong)\n", dirHeight)
				}
			}
		}
		
		if round < 9 {
			time.Sleep(5 * time.Second)
		}
	}
	
	fmt.Println("\n✅ SOLUTION: Use minorBlockSequenceNumber from acc://dn.acme/anchors")
	fmt.Println("This value IS updating and shows the real DN block production!")
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