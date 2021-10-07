package abci_test

import (
	"crypto/sha256"
	"testing"

	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto"
	randpkg "golang.org/x/exp/rand"
)

var rand = randpkg.New(randpkg.NewSource(0))

type Tx = transactions.GenTransaction

func TestE2E_Accumulator_AnonToken(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})
	originAddr := n.anonTokenTest(11)

	t.Log(mustJSON(t, n.GetChainState(originAddr, nil)))
}

func TestE2E_Accumulator_ADI(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})

	anonSponsor := generateKey()
	anonAccount := generateKey()
	newAdi := generateKey()

	n.Batch(func(send func(*Tx)) {
		tx, err := acctesting.CreateFakeSyntheticDeposit(anonSponsor, anonAccount)
		require.NoError(n.t, err)
		send(tx)
	})

	wallet := new(transactions.WalletEntry)
	wallet.Nonce = 1
	wallet.PrivateKey = anonAccount.Bytes()
	wallet.Addr = anon.GenerateAcmeAddress(anonAccount.PubKey().Bytes())

	n.Batch(func(send func(*Tx)) {
		adi := new(api.ADI)
		adi.URL = "RoadRunner"
		adi.PublicKeyHash = sha256.Sum256(newAdi.PubKey().Address())

		sponsorUrl := anon.GenerateAcmeAddress(anonAccount.PubKey().Bytes())
		tx, err := transactions.New(sponsorUrl, func(hash []byte) (*transactions.ED25519Sig, error) {
			return wallet.Sign(hash), nil
		}, adi)
		require.NoError(t, err)

		send(tx)
	})

	n.client.Wait()

	t.Log(mustJSON(t, n.GetChainState("RoadRunner", nil)))
}

func BenchmarkE2E_Accumulator_AnonToken(b *testing.B) {
	n := createAppWithMemDB(b, crypto.Address{})

	sponsor := generateKey()
	recipient := generateKey()

	n.Batch(func(send func(*Tx)) {
		tx, err := acctesting.CreateFakeSyntheticDeposit(sponsor, recipient)
		require.NoError(b, err)
		send(tx)
	})

	origin := transactions.NewWalletEntry()
	origin.Nonce = 1
	origin.PrivateKey = recipient.Bytes()
	origin.Addr = anon.GenerateAcmeAddress(recipient.PubKey().Address())

	rwallet := transactions.NewWalletEntry()

	b.ResetTimer()
	n.Batch(func(send func(*Tx)) {
		for i := 0; i < b.N; i++ {
			output := transactions.Output{Dest: rwallet.Addr, Amount: 1000}
			exch := transactions.NewTokenSend(origin.Addr, output)
			tx, err := transactions.New(origin.Addr, func(hash []byte) (*transactions.ED25519Sig, error) {
				return origin.Sign(hash), nil
			}, exch)
			require.NoError(b, err)
			send(tx)
		}
	})
}

func (n *fakeNode) anonTokenTest(count int) string {
	sponsor := generateKey()
	recipient := generateKey()

	n.Batch(func(send func(*Tx)) {
		tx, err := acctesting.CreateFakeSyntheticDeposit(sponsor, recipient)
		require.NoError(n.t, err)
		send(tx)
	})

	origin := transactions.NewWalletEntry()
	origin.Nonce = 1
	origin.PrivateKey = recipient.Bytes()
	origin.Addr = anon.GenerateAcmeAddress(recipient.PubKey().Bytes())

	recipients := make([]*transactions.WalletEntry, 10)
	for i := range recipients {
		recipients[i] = transactions.NewWalletEntry()
	}

	n.Batch(func(send func(*Tx)) {
		for i := 0; i < count; i++ {
			recipient := recipients[rand.Intn(len(recipients))]
			output := transactions.Output{Dest: recipient.Addr, Amount: 1000}
			exch := transactions.NewTokenSend(origin.Addr, output)
			tx, err := transactions.New(origin.Addr, func(hash []byte) (*transactions.ED25519Sig, error) {
				return origin.Sign(hash), nil
			}, exch)
			require.NoError(n.t, err)
			send(tx)
		}
	})

	n.client.Wait()

	return origin.Addr
}
