package protocol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

const AcmeFaucetAmount = 2_000_000

const AcmeFaucetBalance = "314159265358979323846264338327950288419716939937510582097494459"

var faucetSeed = sha256.Sum256([]byte("faucet"))
var faucetKey = ed25519.NewKeyFromSeed(faucetSeed[:])

var Faucet faucet
var FaucetUrl = liteTokenAddress(Faucet.PublicKey(), AcmeUrl())

// TODO Set the balance to 0 and/or use a bogus URL for the faucet. Otherwise, a
// bad actor could generate the faucet private key using the same method we do,
// then sign arbitrary transactions using the faucet.

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

func (s faucetSigner) Timestamp() uint64 {
	return uint64(s)
}

func (s faucetSigner) PublicKey() []byte {
	return faucetKey[32:]
}

func (s faucetSigner) Sign(message []byte) *LegacyED25519Signature {
	sig := new(LegacyED25519Signature)
	sig.Timestamp = s.Timestamp()
	sig.PublicKey = s.PublicKey()
	sig.Signer = FaucetUrl
	sig.SignerVersion = 1
	SignLegacyED25519(sig, faucetKey, message)
	return sig
}

func (s faucetSigner) Initiate(txn *Transaction) *LegacyED25519Signature {
	sig := new(LegacyED25519Signature)
	sig.Timestamp = s.Timestamp()
	sig.PublicKey = s.PublicKey()
	sig.Signer = FaucetUrl
	sig.SignerVersion = 1

	init, _ := sig.InitiatorHash()
	txn.Header.Initiator = *(*[32]byte)(init)

	SignLegacyED25519(sig, faucetKey, txn.GetHash())
	return sig
}
