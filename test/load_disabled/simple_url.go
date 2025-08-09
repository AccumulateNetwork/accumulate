package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	fmt.Println("🔍 Simple URL Validation Test")
	
	// Generate key
	seed := make([]byte, 32)
	_, err := rand.Read(seed)
	if err != nil {
		log.Fatalf("Failed to generate seed: %v", err)
	}
	
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey[32:]
	
	fmt.Printf("Public key: %x\n", publicKey)
	
	// Create lite token address
	liteAddr, err := protocol.LiteTokenAddress(publicKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("Failed to create lite address: %v", err)
	}
	
	fmt.Printf("Generated URL: %s\n", liteAddr.String())
	
	// Test parsing
	parsedURL, err := url.Parse(liteAddr.String())
	if err != nil {
		fmt.Printf("❌ URL parsing failed: %v\n", err)
		return
	}
	
	fmt.Printf("✅ URL parsing succeeded\n")
	fmt.Printf("Authority: %s\n", parsedURL.Authority)
	fmt.Printf("Path: %s\n", parsedURL.Path)
	fmt.Printf("Full URL: %s\n", parsedURL.String())
	
	// Get the identity
	identity := parsedURL.Identity()
	rootIdentity := parsedURL.RootIdentity()
	
	fmt.Printf("Identity: %s\n", identity.String())
	fmt.Printf("Root Identity: %s\n", rootIdentity.String())
	
	// Check if the URL is valid by seeing if we can parse it back
	re_, err := url.Parse(parsedURL.String())
	if err != nil {
		fmt.Printf("❌ Re-parsing failed: %v\n", err)
	} else {
		fmt.Printf("✅ Re-parsing succeeded\n")
		fmt.Printf("Original:  %s\n", parsedURL.String())
		fmt.Printf("Reparsed: %s\n", reparsed.String())
		fmt.Printf("Match: %t\n", parsedURL.String() == reparsed.String())
	}
	
	// Test some known good URL formats
	fmt.Println("\n🔍 Testing known URL formats...")
	
	testURLs := []string{
		"acc://test.acme",
		"acc://test.acme/tokens",
		"acc://adi.acme/book/1",
	}
	
	for _, testURL := range testURLs {
		_, err := url.Parse(testURL)
		if err != nil {
			fmt.Printf("❌ %s failed: %v\n", testURL, err)
		} else {
			fmt.Printf("✅ %s parsed successfully\n", testURL)
		}
	}
	
	fmt.Println("\n🏁 URL validation test completed!")
}