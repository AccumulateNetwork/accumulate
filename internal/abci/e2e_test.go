package abci_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"testing"

	accapi "github.com/AccumulateNetwork/accumulated/internal/api"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
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

func BenchmarkFaucetAndAnonTx(b *testing.B) {
	n := createAppWithMemDB(b, crypto.Address{})

	sponsor := generateKey()
	recipient := generateKey()

	n.Batch(func(send func(*Tx)) {
		tx, err := acctesting.CreateFakeSyntheticDepositTx(sponsor, recipient)
		require.NoError(b, err)
		send(tx)
	})

	origin := accapi.NewWalletEntry()
	origin.Nonce = 1
	origin.PrivateKey = recipient.Bytes()
	origin.Addr = anon.GenerateAcmeAddress(recipient.PubKey().Address())

	rwallet := accapi.NewWalletEntry()

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

func TestCreateAnonAccount(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})
	originAddr := n.testAnonTx(11)
	require.Equal(t, int64(5e4*acctesting.TokenMx-11000), n.GetAnonTokenAccount(originAddr).Balance.Int64())
}

func (n *fakeNode) testAnonTx(count int) string {
	sponsor := generateKey()
	_, recipient, gtx, err := acctesting.BuildTestSynthDepositGenTx(sponsor.Bytes())
	require.NoError(n.t, err)

	n.Batch(func(send func(*transactions.GenTransaction)) {
		send(gtx)
	})

	origin := accapi.NewWalletEntry()
	origin.Nonce = 1
	origin.PrivateKey = recipient
	origin.Addr = anon.GenerateAcmeAddress(recipient.Public().(ed25519.PublicKey))

	recipients := make([]*transactions.WalletEntry, 10)
	for i := range recipients {
		recipients[i] = accapi.NewWalletEntry()
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

func TestCreateADI(t *testing.T) {
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
	require.Equal(t, types.String("acc://RoadRunner"), r.ChainUrl)
	require.Equal(t, types.Bytes(keyHash[:]), r.KeyData)

	kg := n.GetSigSpecGroup("RoadRunner/ssg0")
	require.Len(t, kg.SigSpecs, 1)

	ks := n.GetSigSpec("RoadRunner/sigspec0")
	require.Len(t, ks.Keys, 1)
	require.Equal(t, keyHash[:], ks.Keys[0].PublicKey)
}

func TestCreateAdiTokenAccount(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})
	adiKey := generateKey()
	require.NoError(t, acctesting.CreateADI(n.db, adiKey, "FooBar"))

	n.Batch(func(send func(*transactions.GenTransaction)) {
		acctTx := api.NewTokenAccount("FooBar/Baz", types.String(protocol.AcmeUrl().String()))
		tx, err := transactions.New("FooBar", edSigner(adiKey, 1), acctTx)
		require.NoError(t, err)
		send(tx)
	})

	n.client.Wait()

	r := n.GetTokenAccount("FooBar/Baz")
	require.Equal(t, types.ChainTypeTokenAccount, r.Type)
	require.Equal(t, types.String("acc://FooBar/Baz"), r.ChainUrl)
	require.Equal(t, types.String(protocol.AcmeUrl().String()), r.TokenUrl.String)
}

func TestAnonAccountTx(t *testing.T) {
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

	require.Equal(t, int64(5e4*acctesting.TokenMx-3000), n.GetAnonTokenAccount(aliceUrl).Balance.Int64())
	require.Equal(t, int64(1000), n.GetAnonTokenAccount(bobUrl).Balance.Int64())
	require.Equal(t, int64(2000), n.GetAnonTokenAccount(charlieUrl).Balance.Int64())
}

func TestAdiAccountTx(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})
	fooKey, barKey := generateKey(), generateKey()
	require.NoError(t, acctesting.CreateADI(n.db, fooKey, "foo"))
	require.NoError(t, acctesting.CreateTokenAccount(n.db, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
	require.NoError(t, acctesting.CreateADI(n.db, barKey, "bar"))
	require.NoError(t, acctesting.CreateTokenAccount(n.db, "bar/tokens", protocol.AcmeUrl().String(), 0, false))

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

func TestSendCreditsFromAdiAccountToMultiSig(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})
	fooKey := generateKey()
	require.NoError(t, acctesting.CreateADI(n.db, fooKey, "foo"))
	require.NoError(t, acctesting.CreateTokenAccount(n.db, "foo/tokens", protocol.AcmeUrl().String(), 1e2, false))

	n.Batch(func(send func(*transactions.GenTransaction)) {
		ac := new(protocol.AddCredits)
		ac.Amount = 55
		ac.Recipient = "foo/sigspec0"

		tx, err := transactions.New("foo/tokens", edSigner(fooKey, 1), ac)
		require.NoError(t, err)
		send(tx)
	})

	n.client.Wait()

	ks := n.GetSigSpec("foo/sigspec0")
	acct := n.GetTokenAccount("foo/tokens")
	require.Equal(t, int64(55), ks.CreditBalance.Int64())
	require.Equal(t, int64(protocol.AcmePrecision*1e2-protocol.AcmePrecision/protocol.CreditsPerDollar*55), acct.Balance.Int64())
}

func TestCreateSigSpec(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})
	fooKey, testKey := generateKey(), generateKey()
	require.NoError(t, acctesting.CreateADI(n.db, fooKey, "foo"))

	n.Batch(func(send func(*transactions.GenTransaction)) {
		cms := new(protocol.CreateSigSpec)
		cms.Url = "foo/keyset1"
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			PublicKey: testKey.PubKey().Bytes(),
		})

		tx, err := transactions.New("foo", edSigner(fooKey, 1), cms)
		require.NoError(t, err)
		send(tx)
	})

	n.client.Wait()
	spec := n.GetSigSpec("foo/keyset1")
	require.Len(t, spec.Keys, 1)
	key := spec.Keys[0]
	require.Equal(t, types.Bytes32{}, spec.SigSpecId)
	require.Equal(t, uint64(0), key.Nonce)
	require.Equal(t, testKey.PubKey().Bytes(), key.PublicKey)
}

func TestCreateSigSpecGroup(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})
	fooKey, testKey := generateKey(), generateKey()
	require.NoError(t, acctesting.CreateADI(n.db, fooKey, "foo"))
	require.NoError(t, acctesting.CreateSigSpec(n.db, "foo/sigspec1", testKey.PubKey().Bytes()))

	specUrl, err := url.Parse("foo/sigspec1")
	require.NoError(t, err)
	specChainId := types.Bytes(specUrl.ResourceChain()).AsBytes32()

	groupUrl, err := url.Parse("foo/ssg1")
	require.NoError(t, err)
	groupChainId := types.Bytes(groupUrl.ResourceChain()).AsBytes32()

	n.Batch(func(send func(*transactions.GenTransaction)) {
		csg := new(protocol.CreateSigSpecGroup)
		csg.Url = "foo/ssg1"
		csg.SigSpecs = append(csg.SigSpecs, specChainId)

		tx, err := transactions.New("foo", edSigner(fooKey, 1), csg)
		require.NoError(t, err)
		send(tx)
	})

	n.client.Wait()
	group := n.GetSigSpecGroup("foo/ssg1")
	require.Len(t, group.SigSpecs, 1)
	require.Equal(t, specChainId, types.Bytes32(group.SigSpecs[0]))

	spec := n.GetSigSpec("foo/sigspec1")
	require.Equal(t, spec.SigSpecId, groupChainId)
}

func TestAddSigSpec(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{})
	fooKey, testKey1, testKey2 := generateKey(), generateKey(), generateKey()

	u, err := url.Parse("foo/ssg1")
	require.NoError(t, err)
	groupChainId := types.Bytes(u.ResourceChain()).AsBytes32()

	require.NoError(t, acctesting.CreateADI(n.db, fooKey, "foo"))
	require.NoError(t, acctesting.CreateSigSpec(n.db, "foo/sigspec1", testKey1.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateSigSpecGroup(n.db, "foo/ssg1", "foo/sigspec1"))

	// Sanity check
	require.Equal(t, groupChainId, n.GetSigSpec("foo/sigspec1").SigSpecId)

	n.Batch(func(send func(*transactions.GenTransaction)) {
		cms := new(protocol.CreateSigSpec)
		cms.Url = "foo/sigspec2"
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			PublicKey: testKey2.PubKey().Bytes(),
		})

		tx, err := transactions.New("foo/ssg1", edSigner(testKey1, 1), cms)
		require.NoError(t, err)
		send(tx)
	})

	n.client.Wait()
	spec := n.GetSigSpec("foo/sigspec2")
	require.Len(t, spec.Keys, 1)
	key := spec.Keys[0]
	require.Equal(t, groupChainId, spec.SigSpecId)
	require.Equal(t, uint64(0), key.Nonce)
	require.Equal(t, testKey2.PubKey().Bytes(), key.PublicKey)
}
