package abci_test

import (
	"crypto/ed25519"

	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func generateKey() tmed25519.PrivKey {
	_, key, _ := ed25519.GenerateKey(rand)
	return tmed25519.PrivKey(key)
}

func edSigner(key tmed25519.PrivKey, nonce uint64) func(hash []byte) (protocol.Signature, error) {
	return func(hash []byte) (protocol.Signature, error) {
		sig := new(protocol.LegacyED25519Signature)
		return sig, sig.Sign(nonce, key, hash)
	}
}
