package genesis

import (
	"crypto/ed25519"
	"crypto/sha256"
	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

var FaucetWallet transactions.WalletEntry
var FaucetUrl *url.URL

func init() {
	FaucetWallet.Nonce = 1

	seed := sha256.Sum256([]byte("faucet"))
	privKey := ed25519.NewKeyFromSeed(seed[:])
	pubKey := privKey.Public().(ed25519.PublicKey)

	FaucetWallet.PrivateKey = privKey
	FaucetUrl, _ = protocol.AnonymousAddress(pubKey, protocol.AcmeUrl().String())
	FaucetWallet.Addr = FaucetUrl.String()
}

func createFaucet() (*types.Bytes32, *state.Object) {
	anon := protocol.NewAnonTokenAccount()

	anon.ChainUrl = types.String(FaucetWallet.Addr)
	anon.TokenUrl = protocol.AcmeUrl().String()
	anon.Balance.SetString("314159265358979323846264338327950288419716939937510582097494459", 10)
	o := new(state.Object)
	acct, err := anon.MarshalBinary()
	if err != nil {
		return nil, nil
	}
	o.Entry = acct
	chainId := types.Bytes(FaucetUrl.ResourceChain()).AsBytes32()
	return &chainId, o
}
