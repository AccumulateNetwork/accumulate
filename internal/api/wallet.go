package api

import (
	"crypto/ed25519"

	lite "github.com/AccumulateNetwork/accumulate/types/anonaddress"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

func NewWalletEntry() *transactions.WalletEntry {
	wallet := new(transactions.WalletEntry)

	wallet.Nonce = 1 // Put the private key for the origin
	_, wallet.PrivateKey, _ = ed25519.GenerateKey(nil)
	wallet.Addr = lite.GenerateAcmeAddress(wallet.PrivateKey[32:]) // Generate the origin address

	return wallet
}
