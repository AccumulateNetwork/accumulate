//go:build ignore
// +build ignore

// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
)

const KNOWN_BAD_HEIGHT = 2460315 // The static/cached value we know is wrong

func main() {
	fmt.Println("🔍 FORCING DN HEIGHT BUG DETECTION")
	fmt.Println("===================================")
	fmt.Printf("Known bad static height: %d\n", KNOWN_BAD_HEIGHT)
	fmt.Println("If we see this value, the bug is NOT fixed!")
	
	// Test mainnet
	mainnetClient, err := client.NewMainnet()
	if err != nil {
		log.Fatal(err)
	}
	
	// Check multiple times to ensure it's static
	var heights []uint64
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		status, err := mainnetClient.GetNetworkStatus(ctx)
		cancel()
		
		if err != nil {
			fmt.Printf("  Check %d: Error: %v\n", i+1, err)
			continue
		}
		
		heights = append(heights, status.DirectoryHeight)
		
		if status.DirectoryHeight == KNOWN_BAD_HEIGHT {
			fmt.Printf("  ❌ Check %d: Height = %d (STATIC BUG DETECTED!)\n", i+1, status.DirectoryHeight)
		} else if i > 0 && status.DirectoryHeight == heights[i-1] {
			fmt.Printf("  ⚠️ Check %d: Height = %d (unchanged from previous)\n", i+1, status.DirectoryHeight)
		} else if i > 0 && status.DirectoryHeight > heights[i-1] {
			fmt.Printf("  ✅ Check %d: Height = %d (+%d) WORKING!\n", i+1, status.DirectoryHeight, status.DirectoryHeight-heights[i-1])
		} else {
			fmt.Printf("  Check %d: Height = %d\n", i+1, status.DirectoryHeight)
		}
		
		time.Sleep(2 * time.Second)
	}
	
	// Final verdict
	allSame := true
	for i := 1; i < len(heights); i++ {
		if heights[i] != heights[0] {
			allSame = false
			break
		}
	}
	
	if len(heights) > 0 && heights[0] == KNOWN_BAD_HEIGHT {
		fmt.Println("\n❌ BUG CONFIRMED: API returns static cached height")
		fmt.Println("The getDnHeight function is NOT working correctly!")
		panic("STATIC HEIGHT BUG - Must continue investigating!")
	} else if allSame {
		fmt.Printf("\n⚠️ Height not changing: %d (may be stuck)\n", heights[0])
	} else {
		fmt.Println("\n✅ Height is changing - bug appears fixed!")
	}
}