// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// keyhash takes an Accumulate-encoded ed25519 private key (AS1...)
// or a hex-encoded ed25519 public key and prints the sha256 hash
// of the public key (the "key hash" used in keypage entries).
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: keyhash <AS1...|hex-pubkey>")
		os.Exit(2)
	}
	in := os.Args[1]

	var pub []byte
	if strings.HasPrefix(in, "AS1") {
		addr, err := address.Parse(in)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse: %v\n", err)
			os.Exit(1)
		}
		priv, ok := addr.GetPrivateKey()
		if !ok {
			fmt.Fprintln(os.Stderr, "not a private key")
			os.Exit(1)
		}
		pub = ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)
	} else {
		var err error
		pub, err = hex.DecodeString(strings.TrimPrefix(in, "0x"))
		if err != nil || len(pub) != 32 {
			fmt.Fprintf(os.Stderr, "hex pubkey must be 64 hex chars (32 bytes), got %d / err=%v\n", len(pub), err)
			os.Exit(1)
		}
	}
	h := sha256.Sum256(pub)
	fmt.Println(hex.EncodeToString(h[:]))
}
