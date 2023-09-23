// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmdutil

import (
	"crypto/ed25519"
	"errors"
	"io/fs"
	"os"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// LoadKey attempts to parse the given string as a secret key address or file.
func LoadKey(s string) ed25519.PrivateKey {
	// Parse the key
	if !strings.ContainsRune(s, '/') {
		sk := parseKey(s)
		if sk != nil {
			return sk
		}
	}

	// If a recognized format, try interpreting it as a file name
	b, err := os.ReadFile(s)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			Fatalf("%q is neither a file nor a recognized key format", s)
		}
		Checkf(err, "read key file")
	}

	sk := parseKey(string(b))
	if sk == nil {
		Fatalf("%q is not a valid key", s)
	}
	return sk
}

func parseKey(s string) ed25519.PrivateKey {
	addr, err := address.Parse(s)
	if err != nil {
		if err.Error() != "unknown address format" {
			Checkf(err, "parse key")
		}
		return nil
	}

	if addr, ok := addr.(*address.Unknown); ok {
		return parseEd25519Key(addr.Value)
	}

	b, ok := addr.GetPrivateKey()
	if !ok {
		// Don't print out the value in case its sensitive
		Fatalf("key is not a private key")
	}

	switch addr.GetType() {
	case protocol.SignatureTypeED25519,
		protocol.SignatureTypeRCD1:
		return parseEd25519Key(b)

	default:
		Fatalf("unsupported key type %v", addr.GetType())
		panic("not reached")
	}
}

func parseEd25519Key(b []byte) ed25519.PrivateKey {
	switch len(b) {
	case ed25519.PrivateKeySize:
		return b
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(b)
	default:
		Fatalf("key is wrong length: want 64 or 32, got %d", len(b))
		panic("not reached")
	}
}
