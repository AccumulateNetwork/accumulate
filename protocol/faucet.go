package protocol

import (
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

var FaucetWallet transactions.WalletEntry
var FaucetUrl *url.URL

// TODO Set the balance to 0 and/or use a bogus URL for the faucet. Otherwise, a
// bad actor could generate the faucet private key using the same method we do,
// then sign arbitrary transactions using the faucet.

func init() {
	FaucetWallet.Nonce = 1

	seed := sha256.Sum256([]byte("faucet"))
	privKey := ed25519.NewKeyFromSeed(seed[:])
	pubKey := privKey.Public().(ed25519.PublicKey)

	var err error
	FaucetUrl, err = LiteAddress(pubKey, AcmeUrl().String())
	if err != nil {
		panic(err)
	}

	FaucetWallet.PrivateKey = privKey
	FaucetWallet.Addr = FaucetUrl.String()
}
