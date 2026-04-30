// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// faucet-key derives the genesis faucet account from one or more
// `--faucet-seed` strings (the same logic used by `accumulated init
// network`). It prints the AS1 private key and the lite token account
// URL (...acme/<addr>/ACME) so a load-test harness can sign as the
// faucet without needing a separate accumulated-faucet daemon.
package main

import (
	"crypto/ed25519"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: faucet-key <seed> [seed...]")
		os.Exit(2)
	}

	var seed storage.Key
	for _, s := range os.Args[1:] {
		seed = seed.Append(s)
	}
	sk := ed25519.NewKeyFromSeed(seed[:])

	u, err := protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	as1 := address.FormatAS1(seed[:])
	fmt.Printf("AS1=%s\n", as1)
	fmt.Printf("URL=%s\n", u)
}
