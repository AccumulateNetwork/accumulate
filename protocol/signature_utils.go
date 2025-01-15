// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import "crypto/sha256"

type SignableHash [32]byte

func (h SignableHash) Hash() [32]byte { return h }

func verifySig(inner, outer Signature, merkle bool, msg interface{ Hash() [32]byte }, verify func([]byte) bool) bool {
	return verifySigSplit(inner, outer, merkle, msg, func(sig, msg []byte) bool {
		return verify(doSha256(sig, msg))
	})
}

func verifySigSplit(inner, outer Signature, merkle bool, msg interface{ Hash() [32]byte }, verify func(_, _ []byte) bool) bool {
	if outer == nil {
		outer = inner
	}
	msgHash := msg.Hash()
	if verify(outer.Metadata().Hash(), msgHash[:]) {
		return true
	}
	if !merkle {
		return false
	}

	us, ok := outer.(UserSignature)
	if !ok {
		return false
	}
	h, err := us.Initiator()
	if err != nil {
		return false
	}
	return verify(h.MerkleHash(), msgHash[:])
}

func signatureHash(sig Signature) []byte {
	// This should never fail unless the signature uses bigints
	data, _ := sig.MarshalBinary()
	return doSha256(data)
}

func signingHash(sig Signature, hasher hashFunc, sigMdHash, txnHash []byte) []byte {
	if sigMdHash == nil {
		sigMdHash = sig.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, txnHash...)
	return hasher(data)
}

type hashFunc func(data ...[]byte) []byte

func doSha256(data ...[]byte) []byte {
	var all []byte
	for _, data := range data {
		all = append(all, data...)
	}
	hash := sha256.Sum256(all)
	return hash[:]
}
