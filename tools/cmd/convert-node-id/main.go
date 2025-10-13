package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multihash"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: convert-node-id <cometbft-node-id>")
		os.Exit(1)
	}

	nodeID := os.Args[1]

	// Decode the CometBFT node ID (20 bytes hex)
	nodeIDBytes, err := hex.DecodeString(nodeID)
	if err != nil {
		fmt.Printf("Error decoding node ID: %v\n", err)
		os.Exit(1)
	}

	if len(nodeIDBytes) != 20 {
		fmt.Printf("Node ID must be 20 bytes (40 hex chars), got %d bytes\n", len(nodeIDBytes))
		os.Exit(1)
	}

	// Pad to 32 bytes (SHA256 output size)
	padded := make([]byte, 32)
	copy(padded, nodeIDBytes)

	// Create SHA2_256 multihash
	mh, err := multihash.Encode(padded, multihash.SHA2_256)
	if err != nil {
		fmt.Printf("Error creating multihash: %v\n", err)
		os.Exit(1)
	}

	// Convert to peer ID
	peerID, err := peer.IDFromBytes(mh)
	if err != nil {
		fmt.Printf("Error creating peer ID: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(peerID.String())
}
