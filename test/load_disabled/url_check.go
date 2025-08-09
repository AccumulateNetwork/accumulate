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
	fmt.Println("🔍 URL Validation Check")

	// Generate key
	seed := make([]byte, 32)
	rand.Read(seed)

	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey[32:]

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
		fmt.Println("This could be the root cause of the signing issue!")
		return
	}

	fmt.Printf("✅ URL parsing succeeded\n")
	fmt.Printf("Authority: %s (length: %d)\n", parsedURL.Authority, len(parsedURL.Authority))
	fmt.Printf("Path: %s\n", parsedURL.Path)

	// Get identities
	identity := parsedURL.Identity()
	rootIdentity := parsedURL.RootIdentity()

	fmt.Printf("Identity: %s\n", identity.String())
	fmt.Printf("Root Identity: %s\n", rootIdentity.String())

	fmt.Println("\n✅ URL appears to be valid for Accumulate!")
}
