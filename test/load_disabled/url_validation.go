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
	fmt.Println("🔍 Accumulate URL Validation Test")
	fmt.Println("Testing if generated lite account URLs are valid")

	// Generate key using the pattern we've been using
	seed := make([]byte, 32)
	_, err := rand.Read(seed)
	if err != nil {
		log.Fatalf("Failed to generate seed: %v", err)
	}

	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey[32:]

	fmt.Printf("Private key length: %d\n", len(privateKey))
	fmt.Printf("Public key length: %d\n", len(publicKey))
	fmt.Printf("Public key: %x\n", publicKey)

	// Create lite token address using our current method
	liteAddr, err := protocol.LiteTokenAddress(publicKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("Failed to create lite address: %v", err)
	}

	fmt.Printf("\n📍 Generated URL: %s\n", liteAddr.String())

	// Test URL parsing and validation
	fmt.Println("\n🔍 Testing URL parsing...")

	// Parse the URL we generated
	parsedURL, err := url.Parse(liteAddr.String())
	if err != nil {
		fmt.Printf("❌ URL parsing failed: %v\n", err)
	} else {
		fmt.Printf("✅ URL parses successfully\n")
		fmt.Printf("   Authority: %s\n", parsedURL.Authority)
		fmt.Printf("   Path: %s\n", parsedURL.Path)
		fmt.Printf("   String: %s\n", parsedURL.String())
	}

	// Test if it's recognized as a lite account
	fmt.Println("\n🔍 Testing lite account recognition...")

	if parsedURL != nil {
		// Check various URL properties
		fmt.Printf("   Identity: %s\n", parsedURL.Identity().String())
		fmt.Printf("   Root Identity: %s\n", parsedURL.RootIdentity().String())

		// Check if it's a lite account
		isLite := len(parsedURL.Authority) == 64 || len(parsedURL.Authority) == 40 // typical lite account lengths
		fmt.Printf("   Appears to be lite account: %t\n", isLite)
		fmt.Printf("   Authority length: %d\n", len(parsedURL.Authority))
	}

	// Test alternative URL generation methods
	fmt.Println("\n🔍 Testing alternative URL generation...")

	// Try different approaches to see if we get different results

	// Method 1: Using our current approach
	fmt.Println("Method 1 (current):")
	addr1, err := protocol.LiteTokenAddress(publicKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		fmt.Printf("❌ Method 1 failed: %v\n", err)
	} else {
		fmt.Printf("   Result: %s\n", addr1.String())
	}

	// Method 2: Try with different token URL format
	fmt.Println("Method 2 (explicit ACME URL):")
	acmeURL := protocol.AcmeUrl()
	fmt.Printf("   ACME URL: %s\n", acmeURL.String())

	addr2, err := protocol.LiteTokenAddress(publicKey, acmeURL.String(), protocol.SignatureTypeED25519)
	if err != nil {
		fmt.Printf("❌ Method 2 failed: %v\n", err)
	} else {
		fmt.Printf("   Result: %s\n", addr2.String())
	}

	// Method 3: Check what the identity should look like
	fmt.Println("Method 3 (compare identity):")
	identity := parsedURL.Identity()
	fmt.Printf("   Parsed Identity: %s\n", identity.String())

	// Validation summary
	fmt.Println("\n📋 Validation Summary:")
	if liteAddr != nil {
		fmt.Printf("✅ Lite token address generated successfully\n")
		fmt.Printf("✅ URL format appears correct: %s\n", liteAddr.String())

		// The key test: does the URL validate properly?
		if parsedURL != nil {
			fmt.Printf("✅ URL parsing successful\n")
		} else {
			fmt.Printf("❌ URL parsing failed - this could be the issue!\n")
		}
	}

	fmt.Println("\n🏁 URL validation test completed!")
}
