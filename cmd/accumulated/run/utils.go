// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"
	"reflect"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func setDefaultPtr[V any](ptr **V, def V) {
	if *ptr == nil {
		*ptr = &def
	}
}

func setDefaultVal[V any](ptr *V, def V) {
	if reflect.ValueOf(ptr).Elem().IsZero() {
		*ptr = def
	}
}

func mustParseMulti(s string) multiaddr.Multiaddr {
	addr, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		panic(err)
	}
	return addr
}

func getPrivateKey(key PrivateKey, inst *Instance) (ed25519.PrivateKey, error) {
	addr, err := key.get(inst)
	if err != nil {
		return nil, err
	}
	if addr.GetType() != protocol.SignatureTypeED25519 {
		return nil, errors.BadRequest.WithFormat("key type %v not supported", addr.GetType())
	}
	sk, ok := addr.GetPrivateKey()
	if !ok {
		return nil, errors.BadRequest.WithFormat("missing private key")
	}
	return sk, nil
}
