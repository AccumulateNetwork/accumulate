package protocol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

const AcmeFaucetAmount = 2000000

var FaucetUrl *url.URL
var Faucet faucet

var faucetSeed = sha256.Sum256([]byte("faucet"))
var faucetKey = ed25519.NewKeyFromSeed(faucetSeed[:])

// TODO Set the balance to 0 and/or use a bogus URL for the faucet. Otherwise, a
// bad actor could generate the faucet private key using the same method we do,
// then sign arbitrary transactions using the faucet.

func init() {
	var err error
	FaucetUrl, err = LiteTokenAddress(Faucet.PublicKey(), AcmeUrl().String())
	if err != nil {
		panic(err)
	}
}

type faucet struct{}

func (faucet) Url() *url.URL {
	return FaucetUrl
}

func (faucet) PublicKey() []byte {
	return faucetKey[32:]
}

func (faucet) Signer() faucetSigner {
	return faucetSigner(time.Now().UnixNano())
}

type faucetSigner uint64

func (s faucetSigner) Nonce() uint64 {
	return uint64(s)
}

func (s faucetSigner) PublicKey() []byte {
	return faucetKey[32:]
}

func SignWithFaucet(nonce uint64, message []byte) (Signature, error) {
	sig := new(LegacyED25519Signature)
	err := sig.Sign(nonce, faucetKey, message)
	return sig, err
}

func (s faucetSigner) Sign(message []byte) (Signature, error) {
	return SignWithFaucet(uint64(s), message)
}
