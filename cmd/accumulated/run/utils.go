// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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
