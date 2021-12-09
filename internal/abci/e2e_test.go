package abci_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	accapi "github.com/AccumulateNetwork/accumulate/internal/api"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/internal/testing/e2e"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	lite "github.com/AccumulateNetwork/accumulate/types/anonaddress"
	"github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/tendermint/tendermint/crypto"
	randpkg "golang.org/x/exp/rand"
)

var rand = randpkg.New(randpkg.NewSource(0))

type Tx = transactions.GenTransaction

func TestEndToEndSuite(t *testing.T) {
	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) e2e.DUT {
		// Recreate the app for each test
		n := createAppWithMemDB(s.T(), crypto.Address{}, true)
		return e2eDUT{n}
	}))
}

func BenchmarkFaucetAndLiteTx(b *testing.B) {
	n := createAppWithMemDB(b, crypto.Address{}, true)

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
	origin.Addr = lite.GenerateAcmeAddress(recipient.PubKey().Address())

	rwallet := accapi.NewWalletEntry()

	b.ResetTimer()
	n.Batch(func(send func(*Tx)) {
		for i := 0; i < b.N; i++ {
			exch := api.NewTokenTx(types.String(origin.Addr))
			exch.AddToAccount(types.String(rwallet.Addr), 1000)
			tx, err := transactions.New(origin.Addr, 1, func(hash []byte) (*transactions.ED25519Sig, error) {
				return origin.Sign(hash), nil
			}, exch)
			require.NoError(b, err)
			send(tx)
		}
	})
}

func TestCreateLiteAccount(t *testing.T) {
	var count = 11
	n := createAppWithMemDB(t, crypto.Address{}, true)
	originAddr, balances := n.testLiteTx(count)
	require.Equal(t, int64(5e4*acctesting.TokenMx-count*1000), n.GetLiteTokenAccount(originAddr).Balance.Int64())
	for addr, bal := range balances {
		require.Equal(t, bal, n.GetLiteTokenAccount(addr).Balance.Int64())
	}
}

func (n *fakeNode) testLiteTx(count int) (string, map[string]int64) {
	sponsor := generateKey()
	_, recipient, gtx, err := acctesting.BuildTestSynthDepositGenTx(sponsor.Bytes())
	require.NoError(n.t, err)

	origin := accapi.NewWalletEntry()
	origin.Nonce = 1
	origin.PrivateKey = recipient
	origin.Addr = lite.GenerateAcmeAddress(recipient.Public().(ed25519.PublicKey))

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
			tx, err := transactions.New(origin.Addr, 1, func(hash []byte) (*transactions.ED25519Sig, error) {
				return origin.Sign(hash), nil
			}, exch)
			require.NoError(n.t, err)
			send(tx)
		}
	})

	return origin.Addr, balance
}

func TestFaucet(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)
	alice := generateKey()
	aliceUrl := lite.GenerateAcmeAddress(alice.PubKey().Bytes())

	n.Batch(func(send func(*transactions.GenTransaction)) {
		body := new(protocol.AcmeFaucet)
		body.Url = aliceUrl
		tx, err := transactions.New(protocol.FaucetUrl.String(), 1, func(hash []byte) (*transactions.ED25519Sig, error) {
			return protocol.FaucetWallet.Sign(hash), nil
		}, body)
		require.NoError(t, err)
		send(tx)
	})

	require.Equal(t, int64(10*protocol.AcmePrecision), n.GetLiteTokenAccount(aliceUrl).Balance.Int64())
}

func TestAnchorChain(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)
	liteAccount := generateKey()
	dbTx := n.db.Begin()
	require.NoError(n.t, acctesting.CreateLiteTokenAccount(dbTx, liteAccount, 5e4))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	n.Batch(func(send func(*Tx)) {
		adi := new(protocol.IdentityCreate)
		adi.Url = "RoadRunner"
		adi.KeyBookName = "book"
		adi.KeyPageName = "page"

		sponsorUrl := lite.GenerateAcmeAddress(liteAccount.PubKey().Bytes())
		tx, err := transactions.New(sponsorUrl, 1, edSigner(liteAccount, 1), adi)
		require.NoError(t, err)

		send(tx)
	})

	// Sanity check
	require.Equal(t, types.String("acc://RoadRunner"), n.GetADI("RoadRunner").ChainUrl)

	// Get the anchor chain manager
	anchor, err := n.db.MinorAnchorChain()
	require.NoError(t, err)

	// Extract and verify the anchor chain head
	head, err := anchor.Record()
	require.NoError(t, err)
	require.ElementsMatch(t, [][32]byte{
		types.Bytes((&url.URL{Authority: "RoadRunner"}).ResourceChain()).AsBytes32(),
		types.Bytes((&url.URL{Authority: "RoadRunner/book"}).ResourceChain()).AsBytes32(),
		types.Bytes((&url.URL{Authority: "RoadRunner/page"}).ResourceChain()).AsBytes32(),
	}, head.Chains)

	// Check each anchor
	first := anchor.Height() - int64(len(head.Chains))
	for i, chain := range head.Chains {
		mgr, err := n.db.ManageChain(chain)
		require.NoError(t, err)

		root, err := anchor.Chain.Entry(first + int64(i))
		require.NoError(t, err)

		assert.Equal(t, mgr.Anchor(), root, "wrong anchor for %X", chain)
	}
}

func TestCreateADI(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)

	liteAccount := generateKey()
	newAdi := generateKey()
	keyHash := sha256.Sum256(newAdi.PubKey().Address())
	dbTx := n.db.Begin()
	require.NoError(n.t, acctesting.CreateLiteTokenAccount(dbTx, liteAccount, 5e4))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	wallet := new(transactions.WalletEntry)
	wallet.Nonce = 1
	wallet.PrivateKey = liteAccount.Bytes()
	wallet.Addr = lite.GenerateAcmeAddress(liteAccount.PubKey().Bytes())

	n.Batch(func(send func(*Tx)) {
		adi := new(protocol.IdentityCreate)
		adi.Url = "RoadRunner"
		adi.PublicKey = keyHash[:]
		adi.KeyBookName = "foo-book"
		adi.KeyPageName = "bar-page"

		sponsorUrl := lite.GenerateAcmeAddress(liteAccount.PubKey().Bytes())
		tx, err := transactions.New(sponsorUrl, 1, func(hash []byte) (*transactions.ED25519Sig, error) {
			return wallet.Sign(hash), nil
		}, adi)
		require.NoError(t, err)

		send(tx)
	})

	r := n.GetADI("RoadRunner")
	require.Equal(t, types.String("acc://RoadRunner"), r.ChainUrl)
	require.Equal(t, types.Bytes(keyHash[:]), r.KeyData)

	kg := n.GetKeyBook("RoadRunner/foo-book")
	require.Len(t, kg.Pages, 1)

	ks := n.GetKeyPage("RoadRunner/bar-page")
	require.Len(t, ks.Keys, 1)
	require.Equal(t, keyHash[:], ks.Keys[0].PublicKey)
}

func TestCreateAdiDataAccount(t *testing.T) {

	t.Run("Data Account w/ Default Key Book and no Manager Key Book", func(t *testing.T) {
		n := createAppWithMemDB(t, crypto.Address{}, true)
		adiKey := generateKey()
		dbTx := n.db.Begin()
		require.NoError(t, acctesting.CreateADI(dbTx, adiKey, "FooBar"))
		dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

		n.Batch(func(send func(*transactions.GenTransaction)) {
			tac := new(protocol.CreateDataAccount)
			tac.Url = "FooBar/oof"
			tx, err := transactions.New("FooBar", 1, edSigner(adiKey, 1), tac)
			require.NoError(t, err)
			send(tx)
		})

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, types.ChainTypeDataAccount, r.Type)
		require.Equal(t, types.String("acc://FooBar/oof"), r.ChainUrl)

		require.Equal(t, []string{
			n.ParseUrl("FooBar/ssg0").String(),
			n.ParseUrl("FooBar/sigspec0").String(),
			n.ParseUrl("FooBar/oof").String(),
		}, n.GetDirectory("FooBar"))
	})

	t.Run("Data Account w/ Custom Key Book and Manager Key Book Url", func(t *testing.T) {
		n := createAppWithMemDB(t, crypto.Address{}, true)
		adiKey, pageKey := generateKey(), generateKey()
		dbTx := n.db.Begin()
		require.NoError(t, acctesting.CreateADI(dbTx, adiKey, "FooBar"))
		require.NoError(t, acctesting.CreateKeyPage(dbTx, "acc://FooBar/foo/page1", pageKey.PubKey().Bytes()))
		require.NoError(t, acctesting.CreateKeyBook(dbTx, "acc://FooBar/foo/book1", "acc://FooBar/foo/page1"))
		require.NoError(t, acctesting.CreateKeyPage(dbTx, "acc://FooBar/mgr/page1", pageKey.PubKey().Bytes()))
		require.NoError(t, acctesting.CreateKeyBook(dbTx, "acc://FooBar/mgr/book1", "acc://FooBar/mgr/page1"))
		dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

		n.Batch(func(send func(*transactions.GenTransaction)) {
			cda := new(protocol.CreateDataAccount)
			cda.Url = "FooBar/oof"
			cda.KeyBookUrl = "acc://FooBar/foo/book1"
			cda.ManagerKeyBookUrl = "acc://FooBar/mgr/book1"
			tx, err := transactions.New("FooBar", 1, edSigner(adiKey, 1), cda)
			require.NoError(t, err)
			send(tx)
		})

		u := n.ParseUrl("acc://FooBar/foo/book1")
		bookChainId := types.Bytes(u.ResourceChain()).AsBytes32()

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, types.ChainTypeDataAccount, r.Type)
		require.Equal(t, types.String("acc://FooBar/oof"), r.ChainUrl)
		require.Equal(t, types.String("acc://FooBar/mgr/book1"), r.ManagerKeyBook)
		require.Equal(t, bookChainId, r.KeyBook)

	})

	t.Run("Data Account data entry", func(t *testing.T) {
		n := createAppWithMemDB(t, crypto.Address{}, true)
		adiKey := generateKey()
		dbTx := n.db.Begin()
		require.NoError(t, acctesting.CreateADI(dbTx, adiKey, "FooBar"))
		dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

		n.Batch(func(send func(*transactions.GenTransaction)) {
			tac := new(protocol.CreateDataAccount)
			tac.Url = "FooBar/oof"
			tx, err := transactions.New("FooBar", 1, edSigner(adiKey, 1), tac)
			require.NoError(t, err)
			send(tx)
		})

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, types.ChainTypeDataAccount, r.Type)
		require.Equal(t, types.String("acc://FooBar/oof"), r.ChainUrl)

		require.Equal(t, []string{
			n.ParseUrl("FooBar/ssg0").String(),
			n.ParseUrl("FooBar/sigspec0").String(),
			n.ParseUrl("FooBar/oof").String(),
		}, n.GetDirectory("FooBar"))

		n.Batch(func(send func(*transactions.GenTransaction)) {
			wd := new(protocol.WriteData)
			for i := 0; i < 10; i++ {
				wd.Entry.ExtIds = append(wd.Entry.ExtIds, []byte(fmt.Sprintf("test id %d", i)))
			}

			wd.Entry.Data = []byte("thequickbrownfoxjumpsoverthelazydog")

			tx, err := transactions.New("FooBar/oof", 1, edSigner(adiKey, 2), wd)
			require.NoError(t, err)
			send(tx)
		})
		time.Sleep(3 * time.Second)
		r2 := n.GetChainDataByUrl("FooBar/oof")
		_ = r2
	})
}

func TestCreateAdiTokenAccount(t *testing.T) {
	t.Run("Default Key Book", func(t *testing.T) {
		n := createAppWithMemDB(t, crypto.Address{}, true)
		adiKey := generateKey()
		dbTx := n.db.Begin()
		require.NoError(t, acctesting.CreateADI(dbTx, adiKey, "FooBar"))
		dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

		n.Batch(func(send func(*transactions.GenTransaction)) {
			tac := new(protocol.TokenAccountCreate)
			tac.Url = "FooBar/Baz"
			tac.TokenUrl = protocol.AcmeUrl().String()
			tx, err := transactions.New("FooBar", 1, edSigner(adiKey, 1), tac)
			require.NoError(t, err)
			send(tx)
		})

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
		n := createAppWithMemDB(t, crypto.Address{}, true)
		adiKey, pageKey := generateKey(), generateKey()
		dbTx := n.db.Begin()
		require.NoError(t, acctesting.CreateADI(dbTx, adiKey, "FooBar"))
		require.NoError(t, acctesting.CreateKeyPage(dbTx, "foo/page1", pageKey.PubKey().Bytes()))
		require.NoError(t, acctesting.CreateKeyBook(dbTx, "foo/book1", "foo/page1"))
		dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

		n.Batch(func(send func(*transactions.GenTransaction)) {
			tac := new(protocol.TokenAccountCreate)
			tac.Url = "FooBar/Baz"
			tac.TokenUrl = protocol.AcmeUrl().String()
			tac.KeyBookUrl = "foo/book1"
			tx, err := transactions.New("FooBar", 1, edSigner(adiKey, 1), tac)
			require.NoError(t, err)
			send(tx)
		})

		u := n.ParseUrl("foo/book1")
		bookChainId := types.Bytes(u.ResourceChain()).AsBytes32()

		r := n.GetTokenAccount("FooBar/Baz")
		require.Equal(t, types.ChainTypeTokenAccount, r.Type)
		require.Equal(t, types.String("acc://FooBar/Baz"), r.ChainUrl)
		require.Equal(t, types.String(protocol.AcmeUrl().String()), r.TokenUrl.String)
		require.Equal(t, bookChainId, r.KeyBook)
	})
}

func TestLiteAccountTx(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)
	alice, bob, charlie := generateKey(), generateKey(), generateKey()
	dbTx := n.db.Begin()
	require.NoError(n.t, acctesting.CreateLiteTokenAccount(dbTx, alice, 5e4))
	require.NoError(n.t, acctesting.CreateLiteTokenAccount(dbTx, bob, 0))
	require.NoError(n.t, acctesting.CreateLiteTokenAccount(dbTx, charlie, 0))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	aliceUrl := lite.GenerateAcmeAddress(alice.PubKey().Bytes())
	bobUrl := lite.GenerateAcmeAddress(bob.PubKey().Bytes())
	charlieUrl := lite.GenerateAcmeAddress(charlie.PubKey().Bytes())

	n.Batch(func(send func(*transactions.GenTransaction)) {
		tokenTx := api.NewTokenTx(types.String(aliceUrl))
		tokenTx.AddToAccount(types.String(bobUrl), 1000)
		tokenTx.AddToAccount(types.String(charlieUrl), 2000)

		tx, err := transactions.New(aliceUrl, 2, edSigner(alice, 1), tokenTx)
		require.NoError(t, err)
		send(tx)
	})

	require.Equal(t, int64(5e4*acctesting.TokenMx-3000), n.GetLiteTokenAccount(aliceUrl).Balance.Int64())
	require.Equal(t, int64(1000), n.GetLiteTokenAccount(bobUrl).Balance.Int64())
	require.Equal(t, int64(2000), n.GetLiteTokenAccount(charlieUrl).Balance.Int64())
}

func TestAdiAccountTx(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)
	fooKey, barKey := generateKey(), generateKey()
	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateTokenAccount(dbTx, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
	require.NoError(t, acctesting.CreateADI(dbTx, barKey, "bar"))
	require.NoError(t, acctesting.CreateTokenAccount(dbTx, "bar/tokens", protocol.AcmeUrl().String(), 0, false))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	n.Batch(func(send func(*transactions.GenTransaction)) {
		tokenTx := api.NewTokenTx("foo/tokens")
		tokenTx.AddToAccount("bar/tokens", 68)

		tx, err := transactions.New("foo/tokens", 1, edSigner(fooKey, 1), tokenTx)
		require.NoError(t, err)
		send(tx)
	})

	require.Equal(t, int64(acctesting.TokenMx-68), n.GetTokenAccount("foo/tokens").Balance.Int64())
	require.Equal(t, int64(68), n.GetTokenAccount("bar/tokens").Balance.Int64())
}

func TestSendCreditsFromAdiAccountToMultiSig(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)
	fooKey := generateKey()
	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateTokenAccount(dbTx, "foo/tokens", protocol.AcmeUrl().String(), 1e2, false))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	n.Batch(func(send func(*transactions.GenTransaction)) {
		ac := new(protocol.AddCredits)
		ac.Amount = 55
		ac.Recipient = "foo/sigspec0"

		tx, err := transactions.New("foo/tokens", 1, edSigner(fooKey, 1), ac)
		require.NoError(t, err)
		send(tx)
	})

	ks := n.GetKeyPage("foo/sigspec0")
	acct := n.GetTokenAccount("foo/tokens")
	require.Equal(t, int64(55), ks.CreditBalance.Int64())
	require.Equal(t, int64(protocol.AcmePrecision*1e2-protocol.AcmePrecision/protocol.CreditsPerFiatUnit*55), acct.Balance.Int64())
}

func TestCreateKeyPage(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)
	fooKey, testKey := generateKey(), generateKey()
	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	n.Batch(func(send func(*transactions.GenTransaction)) {
		cms := new(protocol.CreateKeyPage)
		cms.Url = "foo/keyset1"
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			PublicKey: testKey.PubKey().Bytes(),
		})

		tx, err := transactions.New("foo", 1, edSigner(fooKey, 1), cms)
		require.NoError(t, err)
		send(tx)
	})

	spec := n.GetKeyPage("foo/keyset1")
	require.Len(t, spec.Keys, 1)
	key := spec.Keys[0]
	require.Equal(t, types.Bytes32{}, spec.KeyBook)
	require.Equal(t, uint64(0), key.Nonce)
	require.Equal(t, testKey.PubKey().Bytes(), key.PublicKey)
}

func TestCreateKeyBook(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)
	fooKey, testKey := generateKey(), generateKey()
	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyPage(dbTx, "foo/sigspec1", testKey.PubKey().Bytes()))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	specUrl := n.ParseUrl("foo/sigspec1")
	specChainId := types.Bytes(specUrl.ResourceChain()).AsBytes32()

	groupUrl := n.ParseUrl("foo/ssg1")
	groupChainId := types.Bytes(groupUrl.ResourceChain()).AsBytes32()

	n.Batch(func(send func(*transactions.GenTransaction)) {
		csg := new(protocol.CreateKeyBook)
		csg.Url = "foo/ssg1"
		csg.Pages = append(csg.Pages, specChainId)

		tx, err := transactions.New("foo", 1, edSigner(fooKey, 1), csg)
		require.NoError(t, err)
		send(tx)
	})

	group := n.GetKeyBook("foo/ssg1")
	require.Len(t, group.Pages, 1)
	require.Equal(t, specChainId, types.Bytes32(group.Pages[0]))

	spec := n.GetKeyPage("foo/sigspec1")
	require.Equal(t, spec.KeyBook, groupChainId)
}

func TestAddKeyPage(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)
	fooKey, testKey1, testKey2 := generateKey(), generateKey(), generateKey()

	u := n.ParseUrl("foo/ssg1")
	groupChainId := types.Bytes(u.ResourceChain()).AsBytes32()

	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyPage(dbTx, "foo/sigspec1", testKey1.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyBook(dbTx, "foo/ssg1", "foo/sigspec1"))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	// Sanity check
	require.Equal(t, groupChainId, n.GetKeyPage("foo/sigspec1").KeyBook)

	n.Batch(func(send func(*transactions.GenTransaction)) {
		cms := new(protocol.CreateKeyPage)
		cms.Url = "foo/sigspec2"
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			PublicKey: testKey2.PubKey().Bytes(),
		})

		tx, err := transactions.New("foo/ssg1", 2, edSigner(testKey1, 1), cms)
		require.NoError(t, err)
		send(tx)
	})

	spec := n.GetKeyPage("foo/sigspec2")
	require.Len(t, spec.Keys, 1)
	key := spec.Keys[0]
	require.Equal(t, groupChainId, spec.KeyBook)
	require.Equal(t, uint64(0), key.Nonce)
	require.Equal(t, testKey2.PubKey().Bytes(), key.PublicKey)
}

func TestAddKey(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)
	fooKey, testKey := generateKey(), generateKey()

	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyPage(dbTx, "foo/sigspec1", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyBook(dbTx, "foo/ssg1", "foo/sigspec1"))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	newKey := generateKey()
	n.Batch(func(send func(*transactions.GenTransaction)) {
		body := new(protocol.UpdateKeyPage)
		body.Operation = protocol.AddKey
		body.NewKey = newKey.PubKey().Bytes()

		tx, err := transactions.New("foo/sigspec1", 2, edSigner(testKey, 1), body)
		require.NoError(t, err)
		send(tx)
	})

	spec := n.GetKeyPage("foo/sigspec1")
	require.Len(t, spec.Keys, 2)
	require.Equal(t, newKey.PubKey().Bytes(), spec.Keys[1].PublicKey)
}

func TestUpdateKey(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)
	fooKey, testKey := generateKey(), generateKey()

	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyPage(dbTx, "foo/sigspec1", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyBook(dbTx, "foo/ssg1", "foo/sigspec1"))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	newKey := generateKey()
	n.Batch(func(send func(*transactions.GenTransaction)) {
		body := new(protocol.UpdateKeyPage)
		body.Operation = protocol.UpdateKey
		body.Key = testKey.PubKey().Bytes()
		body.NewKey = newKey.PubKey().Bytes()

		tx, err := transactions.New("foo/sigspec1", 2, edSigner(testKey, 1), body)
		require.NoError(t, err)
		send(tx)
	})

	spec := n.GetKeyPage("foo/sigspec1")
	require.Len(t, spec.Keys, 1)
	require.Equal(t, newKey.PubKey().Bytes(), spec.Keys[0].PublicKey)
}

func TestRemoveKey(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)
	fooKey, testKey1, testKey2 := generateKey(), generateKey(), generateKey()

	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyPage(dbTx, "foo/sigspec1", testKey1.PubKey().Bytes(), testKey2.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyBook(dbTx, "foo/ssg1", "foo/sigspec1"))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	n.Batch(func(send func(*transactions.GenTransaction)) {
		body := new(protocol.UpdateKeyPage)
		body.Operation = protocol.RemoveKey
		body.Key = testKey1.PubKey().Bytes()

		tx, err := transactions.New("foo/sigspec1", 2, edSigner(testKey2, 1), body)
		require.NoError(t, err)
		send(tx)
	})

	spec := n.GetKeyPage("foo/sigspec1")
	require.Len(t, spec.Keys, 1)
	require.Equal(t, testKey2.PubKey().Bytes(), spec.Keys[0].PublicKey)
}

func TestSignatorHeight(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)
	liteKey, fooKey := generateKey(), generateKey()

	liteUrl, err := protocol.LiteAddress(liteKey.PubKey().Bytes(), "ACME")
	require.NoError(t, err)
	tokenUrl, err := url.Parse("foo/tokens")
	require.NoError(t, err)
	keyPageUrl, err := url.Parse("foo/page0")
	require.NoError(t, err)

	dbTx := n.db.Begin()
	require.NoError(t, acctesting.CreateLiteTokenAccount(dbTx, liteKey, 1))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	getHeight := func(u *url.URL) uint64 {
		obj, _, err := n.db.Begin().LoadChain(u.ResourceChain())
		require.NoError(t, err)
		return obj.Height
	}

	liteHeight := getHeight(liteUrl)

	n.Batch(func(send func(*transactions.GenTransaction)) {
		adi := new(protocol.IdentityCreate)
		adi.Url = "foo"
		adi.PublicKey = fooKey.PubKey().Bytes()
		adi.KeyBookName = "book"
		adi.KeyPageName = "page0"

		tx, err := transactions.New(liteUrl.String(), 1, edSigner(liteKey, 1), adi)
		require.NoError(t, err)
		send(tx)
	})

	require.Equal(t, liteHeight, getHeight(liteUrl), "Lite account height changed")

	keyPageHeight := getHeight(keyPageUrl)

	n.Batch(func(send func(*transactions.GenTransaction)) {
		tac := new(protocol.TokenAccountCreate)
		tac.Url = tokenUrl.String()
		tac.TokenUrl = protocol.AcmeUrl().String()
		tx, err := transactions.New("foo", 1, edSigner(fooKey, 1), tac)
		require.NoError(t, err)
		send(tx)
	})

	require.Equal(t, keyPageHeight, getHeight(keyPageUrl), "Key page height changed")
}
