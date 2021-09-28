package security

import (
	"crypto/sha256"
	"math/rand"

	"golang.org/x/crypto/ed25519"
)

// Holds public/private keys used to create tests
type TW map[[32]byte][]byte
type PrivateKey []byte

// PublicKey
// Returns the public key portion of the private key in a [32] byte
func (p PrivateKey) PublicKey() (publicKey [32]byte) {
	copy(publicKey[:], p[32:])
	return publicKey
}

var TestWallet TW

const (
	totalKeys = 500 // Total keys in a wallet
	maxKeys   = 100 // maximum keys a test can request
)

// Initialize the TestWallet
func init() {
	SeedKey := sha256.Sum256([]byte("test wallet"))
	TestWallet = make(map[[32]byte][]byte)
	for i := 0; i < totalKeys; i++ {
		SeedKey = sha256.Sum256(SeedKey[:])
		pk := ed25519.NewKeyFromSeed(SeedKey[:]) // Public Key is the last 32 bytes of pk
		var pub [32]byte
		copy(pub[:], pk[32:])
		TestWallet[pub] = pk
	}
}

// GetKeys
// Return a list of PrivateKeys.  We use a really crude means of
// randomization to return different lists.  It's good enough for
// testing. Note Maps randomize some by their nature, but skipping through the
// wallet helps.  Overuse of some keys doesn't make much difference.
func (w TW) GetKeys(count int) (privateKeys []PrivateKey) {
	if count > maxKeys {
		panic("asking for more keys than the TestWallet has")
	}
	for _, privateKey := range TestWallet { //                Go through our testWallet
		if rand.Int()%10 < 5 { //                             flip a coin.  Use the next key half the time-ish
			privateKeys = append(privateKeys, privateKey) //  Append to our list
		}
		if len(privateKeys) == count { //                     If we have enough keys, return the solution.
			return privateKeys
		}
	}
	if len(privateKeys) < count { // Because we could be unlucky, check that
		return w.GetKeys(count) //   we got enough keys, and try again if we did not.
	}
	return privateKeys //            Return our solution.
}
