package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"crypto/rand"
	"encoding/hex"
)

func main() {
	client := &http.Client{Timeout: 10 * time.Second}
	baseURL := "http://127.0.0.1:26660/v2"
	
	fmt.Println("=== FORCING CROSSCHAIN ACTIVITY ===")
	
	// Create ADIs that should hash to different partitions
	adis := []string{
		"adi-bvn1-test.acme",     // Should route to BVN1  
		"adi-bvn2-test.acme",     // Should route to BVN2
		"adi-bvn3-test.acme",     // Should route to BVN3
		"adi-bvn4-test.acme",     // Should route to BVN4
		"adi-bvn5-test.acme",     // Should route to BVN5
	}
	
	// Create random keys for each ADI
	for i, adi := range adis {
		fmt.Printf("Creating ADI %s...\n", adi)
		
		// Generate random key
		keyBytes := make([]byte, 32)
		rand.Read(keyBytes)
		publicKey := hex.EncodeToString(keyBytes)
		
		// Create ADI
		createReq := map[string]interface{}{
			"type": "createIdentity",
			"data": map[string]interface{}{
				"url": adi,
				"keyBookUrl": adi + "/book",
				"keyHash": publicKey,
			},
		}
		
		reqBody, _ := json.Marshal(createReq)
		resp, err := client.Post(baseURL+"/tx", "application/json", strings.NewReader(string(reqBody)))
		if err != nil {
			log.Printf("Error creating ADI %s: %v", adi, err)
			continue
		}
		resp.Body.Close()
		
		fmt.Printf("ADI %d created: %s\n", i+1, adi)
		time.Sleep(1 * time.Second)
	}
	
	fmt.Println("\n=== CREATING CROSSCHAIN TRANSACTIONS ===")
	
	// Create crosschain token transfers between ADIs
	for i := 0; i < len(adis)-1; i++ {
		from := adis[i] + "/tokens"
		to := adis[i+1] + "/tokens"
		
		fmt.Printf("Transfer from %s to %s\n", from, to)
		
		transferReq := map[string]interface{}{
			"type": "sendTokens", 
			"data": map[string]interface{}{
				"from": from,
				"to": to,
				"amount": "1000000", // 1 ACME
			},
		}
		
		reqBody, _ := json.Marshal(transferReq)
		resp, err := client.Post(baseURL+"/tx", "application/json", strings.NewReader(string(reqBody)))
		if err != nil {
			log.Printf("Error creating transfer: %v", err)
			continue
		}
		resp.Body.Close()
		
		fmt.Printf("Crosschain transfer created from BVN%d to BVN%d\n", i+1, i+2)
		time.Sleep(2 * time.Second) // Allow time for processing
	}
	
	fmt.Println("\n=== CROSSCHAIN ACTIVITY INITIATED ===")
	fmt.Println("Check devnet logs for conductor activity!")
}