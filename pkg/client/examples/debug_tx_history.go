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
	fmt.Println("🔍 Debug Transaction History")
	fmt.Println("============================")
	
	// Query the last 10 transactions from DN anchors
	resp := makeRequest("https://mainnet.accumulatenetwork.io/v2", "query-tx-history", map[string]interface{}{
		"url":     "acc://dn.acme/anchors",
		"count":   10,
		"fromEnd": true,
	})
	
	fmt.Println("\n📊 Last 10 transactions from DN anchors:")
	if resp != nil {
		if result, ok := resp["result"].(map[string]interface{}); ok {
			if items, ok := result["items"].([]interface{}); ok {
				for i, item := range items {
					fmt.Printf("\n  Transaction %d:\n", i+1)
					if tx, ok := item.(map[string]interface{}); ok {
						// Print raw transaction details
						jsonBytes, _ := json.MarshalIndent(tx, "    ", "  ")
						fmt.Println(string(jsonBytes))
					}
				}
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
	
	client := &http.Client{Timeout: 10 * time.Second}
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