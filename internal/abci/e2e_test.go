package abci_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"testing"
	"time"

	accapi "github.com/AccumulateNetwork/accumulate/internal/api"
	"github.com/AccumulateNetwork/accumulate/internal/genesis"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/internal/testing/e2e"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/managed"
	"github.com/AccumulateNetwork/accumulate/types"
	anon "github.com/AccumulateNetwork/accumulate/types/anonaddress"
	"github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto"
	randpkg "golang.org/x/exp/rand"
)

var rand = randpkg.New(randpkg.NewSource(0))

type Tx = transactions.GenTransaction

func TestEndToEndSuite(t *testing.T) {
	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) *accapi.Query {
		// Recreate the app for each test
		n := createAppWithMemDB(s.T(), crypto.Address{}, "error", false)
		n.app.InitChain(abci.RequestInitChain{
			Time:          time.Now(),
			ChainId:       s.T().Name(),
			AppStateBytes: []byte(`""`),
		})
		return n.query
	}))
}

func BenchmarkFaucetAndAnonTx(b *testing.B) {
	n := createAppWithMemDB(b, crypto.Address{}, "error", true)

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
	var count = 11
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)
	originAddr, balances := n.testAnonTx(count)
	require.Equal(t, int64(5e4*acctesting.TokenMx-count*1000), n.GetAnonTokenAccount(originAddr).Balance.Int64())
	for addr, bal := range balances {
		require.Equal(t, bal, n.GetAnonTokenAccount(addr).Balance.Int64())
	}
}

func (n *fakeNode) testAnonTx(count int) (string, map[string]int64) {
	sponsor := generateKey()
	_, recipient, gtx, err := acctesting.BuildTestSynthDepositGenTx(sponsor.Bytes())
	require.NoError(n.t, err)

	origin := accapi.NewWalletEntry()
	origin.Nonce = 1
	origin.PrivateKey = recipient
	origin.Addr = anon.GenerateAcmeAddress(recipient.Public().(ed25519.PublicKey))

	recipients := make([]*transactions.WalletEntry, 10)
	for i := range recipients {
		recipients[i] = accapi.NewWalletEntry()
	}

	n.Batch(func(send func(*transactions.GenTransaction)) {
		send(gtx)
	})

	balance := map[string]int64{}
	n.Batch(func(send func(*Tx)) {
		for i := 0; i < count; i++ {
			recipient := recipients[rand.Intn(len(recipients))]
			balance[recipient.Addr] += 1000

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

	return origin.Addr, balance
}

func TestFaucet(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)
	alice := generateKey()
	aliceUrl := anon.GenerateAcmeAddress(alice.PubKey().Bytes())

	n.Batch(func(send func(*transactions.GenTransaction)) {
		body := new(protocol.AcmeFaucet)
		body.Url = aliceUrl
		tx, err := transactions.New(genesis.FaucetUrl.String(), func(hash []byte) (*transactions.ED25519Sig, error) {
			return genesis.FaucetWallet.Sign(hash), nil
		}, body)
		require.NoError(t, err)
		send(tx)
	})

	n.client.Wait()

	require.Equal(t, int64(10*protocol.AcmePrecision), n.GetAnonTokenAccount(aliceUrl).Balance.Int64())
}

func TestAnchorChain(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)
	anonAccount := generateKey()
	dbTx := n.db.Begin()
	require.NoError(n.t, acctesting.CreateAnonTokenAccount(dbTx, anonAccount, 5e4))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

	n.Batch(func(send func(*Tx)) {
		adi := new(protocol.IdentityCreate)
		adi.Url = "RoadRunner"
		adi.KeyBookName = "book"
		adi.KeyPageName = "page"

		sponsorUrl := anon.GenerateAcmeAddress(anonAccount.PubKey().Bytes())
		tx, err := transactions.New(sponsorUrl, edSigner(anonAccount, 1), adi)
		require.NoError(t, err)

		send(tx)
	})

	// Sanity check
	require.Equal(t, types.String("acc://RoadRunner"), n.GetADI("RoadRunner").ChainUrl)

	// Construct a Merkle manager for the anchor chain
	anchorMM, err := managed.NewMerkleManager(n.db.GetDB(), 0)
	require.NoError(t, err)
	require.NoError(t, anchorMM.SetChainID([]byte("MinorAnchorChain")))

	// Extract and verify the anchor chain head
	head := new(state.AnchorMetadata)
	data, err := anchorMM.Get(anchorMM.MS.Count - 1)
	require.NoError(t, err)
	require.NoError(t, head.UnmarshalBinary(data))
	require.ElementsMatch(t, [][32]byte{
		types.Bytes((&url.URL{Authority: "RoadRunner"}).ResourceChain()).AsBytes32(),
		types.Bytes((&url.URL{Authority: "RoadRunner/book"}).ResourceChain()).AsBytes32(),
		types.Bytes((&url.URL{Authority: "RoadRunner/page"}).ResourceChain()).AsBytes32(),
	}, head.Chains)

	// Check each anchor
	chainMM, err := managed.NewMerkleManager(n.db.GetDB(), 0)
	require.NoError(t, err)
	for i, chain := range head.Chains {
		height := anchorMM.MS.Count - int64(len(head.Chains)) + int64(i)
		root, err := anchorMM.Get(height - 1)
		require.NoError(t, err)

		require.NoError(t, chainMM.SetChainID(chain[:]))
		assert.Equal(t, chainMM.MS.GetMDRoot(), root, "wrong anchor for %X", chain)
	}
}

func TestCreateADI(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)

	anonAccount := generateKey()
	newAdi := generateKey()
	keyHash := sha256.Sum256(newAdi.PubKey().Address())
	dbTx := n.db.Begin()
	require.NoError(n.t, acctesting.CreateAnonTokenAccount(dbTx, anonAccount, 5e4))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

	wallet := new(transactions.WalletEntry)
	wallet.Nonce = 1
	wallet.PrivateKey = anonAccount.Bytes()
	wallet.Addr = anon.GenerateAcmeAddress(anonAccount.PubKey().Bytes())

	n.Batch(func(send func(*Tx)) {
		adi := new(protocol.IdentityCreate)
		adi.Url = "RoadRunner"
		adi.PublicKey = keyHash[:]
		adi.KeyBookName = "foo-book"
		adi.KeyPageName = "bar-page"

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

	kg := n.GetSigSpecGroup("RoadRunner/foo-book")
	require.Len(t, kg.SigSpecs, 1)

	ks := n.GetSigSpec("RoadRunner/bar-page")
	require.Len(t, ks.Keys, 1)
	require.Equal(t, keyHash[:], ks.Keys[0].PublicKey)
}

func TestCreateAdiTokenAccount(t *testing.T) {
	t.Run("Default Key Book", func(t *testing.T) {
		n := createAppWithMemDB(t, crypto.Address{}, "error", true)
		adiKey := generateKey()
		dbTx := n.db.Begin()
		require.NoError(t, acctesting.CreateADI(dbTx, adiKey, "FooBar"))
		dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

		n.Batch(func(send func(*transactions.GenTransaction)) {
			tac := new(protocol.TokenAccountCreate)
			tac.Url = "FooBar/Baz"
			tac.TokenUrl = protocol.AcmeUrl().String()
			tx, err := transactions.New("FooBar", edSigner(adiKey, 1), tac)
			require.NoError(t, err)
			send(tx)
		})

		n.client.Wait()

		r := n.GetTokenAccount("FooBar/Baz")
		require.Equal(t, types.ChainTypeTokenAccount, r.Type)
		require.Equal(t, types.String("acc://FooBar/Baz"), r.ChainUrl)
		require.Equal(t, types.String(protocol.AcmeUrl().String()), r.TokenUrl.String)

		require.Equal(t, []string{
			n.ParseUrl("FooBar/ssg0").String(),
			n.ParseUrl("FooBar/sigspec0").String(),
			n.ParseUrl("FooBar/Baz").String(),
		}, n.GetDirectory("FooBar"))
	})

	t.Run("Custom Key Book", func(t *testing.T) {
		n := createAppWithMemDB(t, crypto.Address{}, "error", true)
		adiKey, pageKey := generateKey(), generateKey()
		dbTx := n.db.Begin()
		require.NoError(t, acctesting.CreateADI(dbTx, adiKey, "FooBar"))
		require.NoError(t, acctesting.CreateSigSpec(dbTx, "foo/page1", pageKey.PubKey().Bytes()))
		require.NoError(t, acctesting.CreateSigSpecGroup(dbTx, "foo/book1", "foo/page1"))
		dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

		n.Batch(func(send func(*transactions.GenTransaction)) {
			tac := new(protocol.TokenAccountCreate)
			tac.Url = "FooBar/Baz"
			tac.TokenUrl = protocol.AcmeUrl().String()
			tac.KeyBookUrl = "foo/book1"
			tx, err := transactions.New("FooBar", edSigner(adiKey, 1), tac)
			require.NoError(t, err)
			send(tx)
		})

		n.client.Wait()

		u := n.ParseUrl("foo/book1")
		bookChainId := types.Bytes(u.ResourceChain()).AsBytes32()

		r := n.GetTokenAccount("FooBar/Baz")
		require.Equal(t, types.ChainTypeTokenAccount, r.Type)
		require.Equal(t, types.String("acc://FooBar/Baz"), r.ChainUrl)
		require.Equal(t, types.String(protocol.AcmeUrl().String()), r.TokenUrl.String)
		require.Equal(t, bookChainId, r.SigSpecId)
	})
}

func TestAnonAccountTx(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)
	alice, bob, charlie := generateKey(), generateKey(), generateKey()
	dbTx := n.db.Begin()
	require.NoError(n.t, acctesting.CreateAnonTokenAccount(dbTx, alice, 5e4))
	require.NoError(n.t, acctesting.CreateAnonTokenAccount(dbTx, bob, 0))
	require.NoError(n.t, acctesting.CreateAnonTokenAccount(dbTx, charlie, 0))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

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
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)
	fooKey, barKey := generateKey(), generateKey()
	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateTokenAccount(dbTx, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
	require.NoError(t, acctesting.CreateADI(dbTx, barKey, "bar"))
	require.NoError(t, acctesting.CreateTokenAccount(dbTx, "bar/tokens", protocol.AcmeUrl().String(), 0, false))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

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
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)
	fooKey := generateKey()
	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateTokenAccount(dbTx, "foo/tokens", protocol.AcmeUrl().String(), 1e2, false))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

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
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)
	fooKey, testKey := generateKey(), generateKey()
	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

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
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)
	fooKey, testKey := generateKey(), generateKey()
	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateSigSpec(dbTx, "foo/sigspec1", testKey.PubKey().Bytes()))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

	specUrl := n.ParseUrl("foo/sigspec1")
	specChainId := types.Bytes(specUrl.ResourceChain()).AsBytes32()

	groupUrl := n.ParseUrl("foo/ssg1")
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
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)
	fooKey, testKey1, testKey2 := generateKey(), generateKey(), generateKey()

	u := n.ParseUrl("foo/ssg1")
	groupChainId := types.Bytes(u.ResourceChain()).AsBytes32()

	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateSigSpec(dbTx, "foo/sigspec1", testKey1.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateSigSpecGroup(dbTx, "foo/ssg1", "foo/sigspec1"))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

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

func TestAddKey(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)
	fooKey, testKey := generateKey(), generateKey()

	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateSigSpec(dbTx, "foo/sigspec1", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateSigSpecGroup(dbTx, "foo/ssg1", "foo/sigspec1"))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

	newKey := generateKey()
	n.Batch(func(send func(*transactions.GenTransaction)) {
		body := new(protocol.UpdateKeyPage)
		body.Operation = protocol.AddKey
		body.NewKey = newKey.PubKey().Bytes()

		tx, err := transactions.New("foo/sigspec1", edSigner(testKey, 1), body)
		require.NoError(t, err)
		send(tx)
	})

	n.client.Wait()

	spec := n.GetSigSpec("foo/sigspec1")
	require.Len(t, spec.Keys, 2)
	require.Equal(t, newKey.PubKey().Bytes(), spec.Keys[1].PublicKey)
}

func TestUpdateKey(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)
	fooKey, testKey := generateKey(), generateKey()

	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateSigSpec(dbTx, "foo/sigspec1", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateSigSpecGroup(dbTx, "foo/ssg1", "foo/sigspec1"))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

	newKey := generateKey()
	n.Batch(func(send func(*transactions.GenTransaction)) {
		body := new(protocol.UpdateKeyPage)
		body.Operation = protocol.UpdateKey
		body.Key = testKey.PubKey().Bytes()
		body.NewKey = newKey.PubKey().Bytes()

		tx, err := transactions.New("foo/sigspec1", edSigner(testKey, 1), body)
		require.NoError(t, err)
		send(tx)
	})

	n.client.Wait()

	spec := n.GetSigSpec("foo/sigspec1")
	require.Len(t, spec.Keys, 1)
	require.Equal(t, newKey.PubKey().Bytes(), spec.Keys[0].PublicKey)
}

func TestRemoveKey(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, "error", true)
	fooKey, testKey1, testKey2 := generateKey(), generateKey(), generateKey()

	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateSigSpec(dbTx, "foo/sigspec1", testKey1.PubKey().Bytes(), testKey2.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateSigSpecGroup(dbTx, "foo/ssg1", "foo/sigspec1"))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0))

	n.Batch(func(send func(*transactions.GenTransaction)) {
		body := new(protocol.UpdateKeyPage)
		body.Operation = protocol.RemoveKey
		body.Key = testKey1.PubKey().Bytes()

		tx, err := transactions.New("foo/sigspec1", edSigner(testKey2, 1), body)
		require.NoError(t, err)
		send(tx)
	})

	n.client.Wait()

	spec := n.GetSigSpec("foo/sigspec1")
	require.Len(t, spec.Keys, 1)
	require.Equal(t, testKey2.PubKey().Bytes(), spec.Keys[0].PublicKey)
}
