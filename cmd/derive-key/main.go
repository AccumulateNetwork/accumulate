// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
)

func main() {
	var seed storage.Key
	for _, s := range os.Args[1:] {
		seed = seed.Append(s)
	}
	sk := ed25519.NewKeyFromSeed(seed[:])
	fmt.Println(hex.EncodeToString(sk))
}
