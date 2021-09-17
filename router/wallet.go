package router

import (
	"crypto/ed25519"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/proto"
)

type walletEntry struct {
	PrivateKey ed25519.PrivateKey // 32 bytes private key, 32 bytes public key
	Nonce      uint64             // Nonce for the signature
	Addr       string             // The address url for the anonymous token chain
}

// Sign
// Makes it easier to sign transactions.  Create the ED25519Sig object, sign
// the message, and return the ED25519Sig object to caller
func (we *walletEntry) Sign(message []byte) *proto.ED25519Sig { // sign a message
	we.Nonce++                       //                            Everytime we sign, increment the nonce
	sig := new(proto.ED25519Sig)     //                            create a signature object
	sig.Nonce = we.Nonce             //                            use the updated nonce
	sig.Sign(we.PrivateKey, message) //                            sign the message
	return sig                       //                            return the signature object
}

func (we *walletEntry) Public() []byte {
	return we.PrivateKey[32:]
}

func NewWalletEntry() *walletEntry {
	wallet := new(walletEntry)

	wallet.Nonce = 1 // Put the private key for the origin
	_, wallet.PrivateKey, _ = ed25519.GenerateKey(nil)
	wallet.Addr = anon.GenerateAcmeAddress(wallet.PrivateKey[32:]) // Generate the origin address

	return wallet
}
