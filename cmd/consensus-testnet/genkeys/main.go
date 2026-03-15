// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// genkeys generates keys and peer IDs for consensus testnet nodes.
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"flag"
	"fmt"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func main() {
	n := flag.Int("n", 7, "Number of nodes")
	flag.Parse()

	fmt.Println("# Generated keys for consensus testnet")
	fmt.Println()

	var seeds []string
	var pubkeys []string
	var peerIDs []string

	for i := 1; i <= *n; i++ {
		// Generate deterministic seed from node number
		seed := make([]byte, 32)
		seed[31] = byte(i)
		seeds = append(seeds, hex.EncodeToString(seed))

		// Derive keys
		privKey := ed25519.NewKeyFromSeed(seed)
		pubKey := privKey.Public().(ed25519.PublicKey)
		pubkeys = append(pubkeys, hex.EncodeToString(pubKey))

		// Derive peer ID
		libp2pKey, _ := crypto.UnmarshalEd25519PrivateKey(privKey)
		peerID, _ := peer.IDFromPrivateKey(libp2pKey)
		peerIDs = append(peerIDs, peerID.String())
	}

	// Print seeds
	fmt.Println("# Seeds (32-byte hex)")
	for i, seed := range seeds {
		fmt.Printf("SEED_%d=%s\n", i+1, seed)
	}
	fmt.Println()

	// Print public keys
	fmt.Println("# Public keys (32-byte hex)")
	for i, pk := range pubkeys {
		fmt.Printf("PUBKEY_%d=%s\n", i+1, pk)
	}
	fmt.Println()

	// Print peer IDs
	fmt.Println("# Peer IDs")
	for i, pid := range peerIDs {
		fmt.Printf("PEERID_%d=%s\n", i+1, pid)
	}
	fmt.Println()

	// Print validators list
	fmt.Println("# Validators (comma-separated pubkeys)")
	fmt.Print("VALIDATORS=")
	for i, pk := range pubkeys {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Print(pk)
	}
	fmt.Println()
	fmt.Println()

	// Print docker-compose environment format
	fmt.Println("# Docker compose peer addresses (for node 1 to connect to)")
	for i := 2; i <= *n; i++ {
		fmt.Printf("# Node %d: /dns4/node%d/tcp/9000/p2p/%s\n", i, i, peerIDs[i-1])
	}
}
