package abci_test

import (
	"crypto/sha256"
	"testing"

	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/types"
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
	require.Equal(t, int64(5e4*acctesting.TokenMx-11000), n.GetTokenAccount(originAddr).Balance.Int64())
}

func (n *fakeNode) anonTokenTest(count int) string {
	recipient := generateKey()
	require.NoError(n.t, acctesting.CreateAnonTokenAccount(n.db, recipient, 5e4))

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
			exch := api.NewTokenTx(types.String(origin.Addr))
			exch.AddToAccount(types.String(recipient.Addr), 1000)
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

func TestE2E_Accumulator_ADI(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})

	anonAccount := generateKey()
	newAdi := generateKey()
	keyHash := sha256.Sum256(newAdi.PubKey().Address())

	require.NoError(n.t, acctesting.CreateAnonTokenAccount(n.db, anonAccount, 5e4))

	wallet := new(transactions.WalletEntry)
	wallet.Nonce = 1
	wallet.PrivateKey = anonAccount.Bytes()
	wallet.Addr = anon.GenerateAcmeAddress(anonAccount.PubKey().Bytes())

	n.Batch(func(send func(*Tx)) {
		adi := new(api.ADI)
		adi.URL = "RoadRunner"
		adi.PublicKeyHash = keyHash

		sponsorUrl := anon.GenerateAcmeAddress(anonAccount.PubKey().Bytes())
		tx, err := transactions.New(sponsorUrl, func(hash []byte) (*transactions.ED25519Sig, error) {
			return wallet.Sign(hash), nil
		}, adi)
		require.NoError(t, err)

		send(tx)
	})

	n.client.Wait()

	r := n.GetADI("RoadRunner")
	require.Equal(t, types.String("RoadRunner"), r.ChainUrl)
	require.Equal(t, types.Bytes(keyHash[:]), r.KeyData)
}

func TestE2E_Accumulator_TokenTx_Anon(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})
	alice, bob, charlie := generateKey(), generateKey(), generateKey()
	require.NoError(n.t, acctesting.CreateAnonTokenAccount(n.db, alice, 5e4))
	require.NoError(n.t, acctesting.CreateAnonTokenAccount(n.db, bob, 0))
	require.NoError(n.t, acctesting.CreateAnonTokenAccount(n.db, charlie, 0))

	aliceUrl := anon.GenerateAcmeAddress(alice.PubKey().Bytes())
	bobUrl := anon.GenerateAcmeAddress(bob.PubKey().Bytes())
	charlieUrl := anon.GenerateAcmeAddress(charlie.PubKey().Bytes())

	n.Batch(func(send func(*transactions.GenTransaction)) {
		tokenTx := api.NewTokenTx(types.String(aliceUrl))
		tokenTx.AddToAccount(types.String(bobUrl), 1000)
		tokenTx.AddToAccount(types.String(charlieUrl), 2000)

		tx, err := transactions.New(aliceUrl, edSigner(alice, 1), tokenTx)
		require.NoError(t, err)
		send(tx)
	})

	n.client.Wait()

	require.Equal(t, int64(5e4*acctesting.TokenMx-3000), n.GetTokenAccount(aliceUrl).Balance.Int64())
	require.Equal(t, int64(1000), n.GetTokenAccount(bobUrl).Balance.Int64())
	require.Equal(t, int64(2000), n.GetTokenAccount(charlieUrl).Balance.Int64())
}

func TestE2E_Accumulator_TokenTx_ADI(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})
	fooKey, barKey := generateKey(), generateKey()
	require.NoError(t, acctesting.CreateADI(n.db, fooKey, "foo"))
	require.NoError(t, acctesting.CreateTokenAccount(n.db, "foo/tokens", "dc/ACME", 1, false))
	require.NoError(t, acctesting.CreateADI(n.db, barKey, "bar"))
	require.NoError(t, acctesting.CreateTokenAccount(n.db, "bar/tokens", "dc/ACME", 0, false))

	n.Batch(func(send func(*transactions.GenTransaction)) {
		tokenTx := api.NewTokenTx("foo/tokens")
		tokenTx.AddToAccount("bar/tokens", 68)

		tx, err := transactions.New("foo/tokens", edSigner(fooKey, 1), tokenTx)
		require.NoError(t, err)
		send(tx)
	})

	n.client.Wait()

	require.Equal(t, int64(acctesting.TokenMx-68), n.GetTokenAccount("foo/tokens").Balance.Int64())
	require.Equal(t, int64(68), n.GetTokenAccount("bar/tokens").Balance.Int64())
}

func TestE2E_Accumulator_TokenAccount_ADI(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})
	adiKey := generateKey()
	require.NoError(t, acctesting.CreateADI(n.db, adiKey, "FooBar"))

	n.Batch(func(send func(*transactions.GenTransaction)) {
		acctTx := api.NewTokenAccount("FooBar/Baz", "dc/ACME")
		tx, err := transactions.New("FooBar", edSigner(adiKey, 1), acctTx)
		require.NoError(t, err)
		send(tx)
	})

	n.client.Wait()

	r := n.GetTokenAccount("FooBar/Baz")
	require.Equal(t, types.ChainTypeTokenAccount, r.Type)
	require.Equal(t, types.String("FooBar/Baz"), r.ChainUrl)
	require.Equal(t, types.String("dc/ACME"), r.TokenUrl.String)
}

func BenchmarkE2E_Accumulator_AnonToken(b *testing.B) {
	n := createAppWithMemDB(b, crypto.Address{})

	sponsor := generateKey()
	recipient := generateKey()

	n.Batch(func(send func(*Tx)) {
		tx, err := acctesting.CreateFakeSyntheticDepositTx(sponsor, recipient)
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
			exch := api.NewTokenTx(types.String(origin.Addr))
			exch.AddToAccount(types.String(rwallet.Addr), 1000)
			tx, err := transactions.New(origin.Addr, func(hash []byte) (*transactions.ED25519Sig, error) {
				return origin.Sign(hash), nil
			}, exch)
			require.NoError(b, err)
			send(tx)
		}
	})
}
