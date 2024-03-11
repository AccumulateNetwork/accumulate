// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci_test

import (
	"crypto/ed25519"

	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

var globalNonce uint64

func generateKey() tmed25519.PrivKey {
	_, key, _ := ed25519.GenerateKey(rand)
	return tmed25519.PrivKey(key)
}

func mustParseOrigin(origin string) *url.URL {
	u, err := acctesting.ParseUrl(origin)
	if err != nil {
		panic(err)
	}
	return u
}
