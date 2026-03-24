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
