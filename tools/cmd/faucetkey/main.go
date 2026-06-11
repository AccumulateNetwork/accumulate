// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Throwaway: reproduce the --faucet-seed key derivation (see
// cmd/accumulated/cmd_init_network.go createFaucet) and print the AS1
// secret key + lite URL so a load tool can drive the genesis-funded
// faucet account directly.
package main

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	seedStr := "FAUCET"
	if len(os.Args) > 1 {
		seedStr = os.Args[1]
	}
	var seed storage.Key
	for _, s := range strings.Split(seedStr, " ") {
		seed = seed.Append(s)
	}
	sk := ed25519.NewKeyFromSeed(seed[:])

	u, err := protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		panic(err)
	}

	priv := &address.PrivateKey{
		PublicKey: address.PublicKey{Type: protocol.SignatureTypeED25519, Key: sk[32:]},
		Key:       sk,
	}
	fmt.Println(priv.String())
	fmt.Println(u)
	fmt.Println(u.RootIdentity())
}
