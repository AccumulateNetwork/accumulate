package abci_test

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/testing/e2e"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
	randpkg "golang.org/x/exp/rand"
)

var rand = randpkg.New(randpkg.NewSource(0))

type Tx = transactions.Envelope

func TestEndToEndSuite(t *testing.T) {
	acctesting.SkipCI(t, "flaky")

	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) e2e.DUT {
		// Recreate the app for each test
		subnets, daemons := acctesting.CreateTestNet(s.T(), 1, 1, 0)
		nodes := RunTestNet(s.T(), subnets, daemons, nil, true)
		n := nodes[subnets[1]][0]

		return &e2eDUT{s, n}
	}))
}

func TestCreateLiteAccount(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	var count = 11
	originAddr, balances := n.testLiteTx(count)
	require.Equal(t, int64(5e4*acctesting.TokenMx-count*1000), n.GetLiteTokenAccount(originAddr).Balance.Int64())
	for addr, bal := range balances {
		require.Equal(t, bal, n.GetLiteTokenAccount(addr).Balance.Int64())
	}
}

func (n *FakeNode) testLiteTx(count int) (string, map[string]int64) {
	_, recipient, gtx, err := acctesting.BuildTestSynthDepositGenTx()
	require.NoError(n.t, err)

	origin := acctesting.NewWalletEntry()
	origin.Nonce = 1
	origin.PrivateKey = recipient
	origin.Addr = acctesting.AcmeLiteAddressStdPriv(recipient).String()

	recipients := make([]*acctesting.WalletEntry, 10)
	for i := range recipients {
		recipients[i] = acctesting.NewWalletEntry()
	}

	n.Batch(func(send func(*transactions.Envelope)) {
		send(gtx)
	})

	batch := n.db.Begin()
	n.Require().NoError(acctesting.AddCredits(batch, acctesting.AcmeLiteAddressStdPriv(recipient), 1e9))
	n.require.NoError(batch.Commit())

	balance := map[string]int64{}
	n.Batch(func(send func(*Tx)) {
		for i := 0; i < count; i++ {
			recipient := recipients[rand.Intn(len(recipients))]
			balance[recipient.Addr] += 1000

			exch := new(protocol.SendTokens)
			exch.AddRecipient(n.ParseUrl(recipient.Addr), big.NewInt(int64(1000)))
			tx, err := transactions.New(origin.Addr, origin.Nonce, func(hash []byte) (*transactions.ED25519Sig, error) {
				return origin.Sign(hash), nil
			}, exch)
			require.NoError(n.t, err)
			send(tx)
			origin.Nonce++
		}
	})

	return origin.Addr, balance
}

func TestFaucet(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	alice := generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)

	n.Batch(func(send func(*transactions.Envelope)) {
		body := new(protocol.AcmeFaucet)
		body.Url = aliceUrl

		faucet := protocol.Faucet.Signer()
		tx, err := transactions.NewWith(&protocol.TransactionHeader{
			Origin:        protocol.FaucetUrl,
			KeyPageHeight: 1,
			Nonce:         faucet.Nonce(),
		}, faucet.Sign, body)
		require.NoError(t, err)
		send(tx)
	})

	require.Equal(t, int64(100*protocol.AcmePrecision), n.GetLiteTokenAccount(aliceUrl.String()).Balance.Int64())
}

func TestAnchorChain(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	liteAccount := generateKey()
	batch := n.db.Begin()
	require.NoError(n.t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteAccount, 5e4, 1e6))
	require.NoError(t, batch.Commit())

	n.Batch(func(send func(*Tx)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = n.ParseUrl("RoadRunner")
		adi.KeyBookName = "book"
		adi.KeyPageName = "page"

		sponsorUrl := acctesting.AcmeLiteAddressTmPriv(liteAccount).String()
		tx, err := transactions.New(sponsorUrl, 1, edSigner(liteAccount, 1), adi)
		require.NoError(t, err)

		send(tx)
	})

	// Sanity check
	require.Equal(t, "acc://RoadRunner", n.GetADI("RoadRunner").Url.String())

	// Get the anchor chain manager
	batch = n.db.Begin()
	defer batch.Discard()
	ledger := batch.Account(n.network.NodeUrl(protocol.Ledger))

	// Check each anchor
	ledgerState := protocol.NewInternalLedger()
	require.NoError(t, ledger.GetStateAs(ledgerState))
	rootChain, err := ledger.ReadChain(protocol.MinorRootChain)
	require.NoError(t, err)
	first := rootChain.Height() - int64(len(ledgerState.Updates))
	for i, meta := range ledgerState.Updates {
		root, err := rootChain.Entry(first + int64(i))
		require.NoError(t, err)

		if meta.Name == "bpt" {
			assert.Equal(t, root, batch.RootHash(), "wrong anchor for BPT")
			continue
		}

		mgr, err := batch.Account(meta.Account).ReadChain(meta.Name)
		require.NoError(t, err)

		assert.Equal(t, root, mgr.Anchor(), "wrong anchor for %s#chain/%s", meta.Account, meta.Name)
	}

	// // TODO Once block indexing has been implemented, verify that the following chains got modified
	// assert.Subset(t, accounts, []string{
	// 	"acc://RoadRunner#chain/main",
	// 	"acc://RoadRunner#chain/pending",
	// 	"acc://RoadRunner/book#chain/main",
	// 	"acc://RoadRunner/page#chain/main",
	// })
}

func TestCreateADI(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	liteAccount := generateKey()
	newAdi := generateKey()
	keyHash := sha256.Sum256(newAdi.PubKey().Address())
	batch := n.db.Begin()
	require.NoError(n.t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteAccount, 5e4, 1e6))
	require.NoError(t, batch.Commit())

	wallet := new(acctesting.WalletEntry)
	wallet.Nonce = 1
	wallet.PrivateKey = liteAccount.Bytes()
	wallet.Addr = acctesting.AcmeLiteAddressTmPriv(liteAccount).String()

	n.Batch(func(send func(*Tx)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = n.ParseUrl("RoadRunner")
		adi.PublicKey = keyHash[:]
		adi.KeyBookName = "foo-book"
		adi.KeyPageName = "bar-page"

		sponsorUrl := acctesting.AcmeLiteAddressTmPriv(liteAccount).String()
		tx, err := transactions.New(sponsorUrl, 1, func(hash []byte) (*transactions.ED25519Sig, error) {
			return wallet.Sign(hash), nil
		}, adi)
		require.NoError(t, err)

		send(tx)
	})

	r := n.GetADI("RoadRunner")
	require.Equal(t, "acc://RoadRunner", r.Url.String())

	kg := n.GetKeyBook("RoadRunner/foo-book")
	require.Len(t, kg.Pages, 1)

	ks := n.GetKeyPage("RoadRunner/bar-page")
	require.Len(t, ks.Keys, 1)
	require.Equal(t, keyHash[:], ks.Keys[0].PublicKey)
}

func TestCreateLiteDataAccount(t *testing.T) {

	//this test exercises WriteDataTo and SyntheticWriteData validators

	firstEntry := protocol.DataEntry{}

	firstEntry.ExtIds = append(firstEntry.ExtIds, []byte("Factom PRO"))
	firstEntry.ExtIds = append(firstEntry.ExtIds, []byte("Tutorial"))

	//create a lite data account aka factom chainId
	chainId := protocol.ComputeLiteDataAccountId(&firstEntry)

	lde := protocol.LiteDataEntry{}
	lde.DataEntry = new(protocol.DataEntry)
	copy(lde.AccountId[:], chainId)
	lde.Data = []byte("This is useful content of the entry. You can save text, hash, JSON or raw ASCII data here.")
	for i := 0; i < 3; i++ {
		lde.ExtIds = append(lde.ExtIds, []byte(fmt.Sprintf("Tag #%d of entry", i+1)))
	}
	liteDataAddress, err := protocol.LiteDataAddress(chainId)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Create ADI then write to Lite Data Account", func(t *testing.T) {
		subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
		nodes := RunTestNet(t, subnets, daemons, nil, true)
		n := nodes[subnets[1]][0]

		adiKey := generateKey()
		batch := n.db.Begin()
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		require.NoError(t, batch.Commit())
		n.Batch(func(send func(*transactions.Envelope)) {
			wdt := new(protocol.WriteDataTo)
			wdt.Recipient = liteDataAddress
			wdt.Entry = firstEntry
			tx, err := transactions.New("FooBar", 1, edSigner(adiKey, 1), wdt)
			require.NoError(t, err)
			send(tx)
		})

		partialChainId, err := protocol.ParseLiteDataAddress(liteDataAddress)
		if err != nil {
			t.Fatal(err)
		}
		r := n.GetLiteDataAccount(liteDataAddress.String())
		require.Equal(t, liteDataAddress.String(), r.Url.String())
		require.Equal(t, append(partialChainId, r.Tail...), chainId)
	})
}

func TestCreateAdiDataAccount(t *testing.T) {

	t.Run("Data Account w/ Default Key Book and no Manager Key Book", func(t *testing.T) {
		subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
		nodes := RunTestNet(t, subnets, daemons, nil, true)
		n := nodes[subnets[1]][0]

		adiKey := generateKey()
		batch := n.db.Begin()
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		require.NoError(t, batch.Commit())

		n.Batch(func(send func(*transactions.Envelope)) {
			tac := new(protocol.CreateDataAccount)
			tac.Url = n.ParseUrl("FooBar/oof")
			tx, err := transactions.New("FooBar", 1, edSigner(adiKey, 1), tac)
			require.NoError(t, err)
			send(tx)
		})

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, "acc://FooBar/oof", r.Url.String())

		require.Contains(t, n.GetDirectory("FooBar"), n.ParseUrl("FooBar/oof").String())
	})

	t.Run("Data Account w/ Custom Key Book and Manager Key Book Url", func(t *testing.T) {
		subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
		nodes := RunTestNet(t, subnets, daemons, nil, true)
		n := nodes[subnets[1]][0]

		adiKey, pageKey := generateKey(), generateKey()
		batch := n.db.Begin()
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		require.NoError(t, acctesting.CreateKeyPage(batch, "acc://FooBar/foo/page1", pageKey.PubKey().Bytes()))
		require.NoError(t, acctesting.CreateKeyBook(batch, "acc://FooBar/foo/book1", "acc://FooBar/foo/page1"))
		require.NoError(t, acctesting.CreateKeyPage(batch, "acc://FooBar/mgr/page1", pageKey.PubKey().Bytes()))
		require.NoError(t, acctesting.CreateKeyBook(batch, "acc://FooBar/mgr/book1", "acc://FooBar/mgr/page1"))
		require.NoError(t, batch.Commit())

		n.Batch(func(send func(*transactions.Envelope)) {
			cda := new(protocol.CreateDataAccount)
			cda.Url = n.ParseUrl("FooBar/oof")
			cda.KeyBookUrl = n.ParseUrl("acc://FooBar/foo/book1")
			cda.ManagerKeyBookUrl = n.ParseUrl("acc://FooBar/mgr/book1")
			tx, err := transactions.New("FooBar", 1, edSigner(adiKey, 1), cda)
			require.NoError(t, err)
			send(tx)
		})

		u := n.ParseUrl("acc://FooBar/foo/book1")

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, "acc://FooBar/oof", r.Url.String())
		require.Equal(t, "acc://FooBar/mgr/book1", r.ManagerKeyBook.String())
		require.Equal(t, u.String(), r.KeyBook.String())

	})

	t.Run("Data Account data entry", func(t *testing.T) {
		subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
		nodes := RunTestNet(t, subnets, daemons, nil, true)
		n := nodes[subnets[1]][0]

		adiKey := generateKey()
		batch := n.db.Begin()
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		require.NoError(t, batch.Commit())

		n.Batch(func(send func(*transactions.Envelope)) {
			tac := new(protocol.CreateDataAccount)
			tac.Url = n.ParseUrl("FooBar/oof")
			tx, err := transactions.New("FooBar", 1, edSigner(adiKey, 1), tac)
			require.NoError(t, err)
			send(tx)
		})

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, "acc://FooBar/oof", r.Url.String())
		require.Contains(t, n.GetDirectory("FooBar"), n.ParseUrl("FooBar/oof").String())

		wd := new(protocol.WriteData)
		n.Batch(func(send func(*transactions.Envelope)) {
			for i := 0; i < 10; i++ {
				wd.Entry.ExtIds = append(wd.Entry.ExtIds, []byte(fmt.Sprintf("test id %d", i)))
			}

			wd.Entry.Data = []byte("thequickbrownfoxjumpsoverthelazydog")

			tx, err := transactions.New("FooBar/oof", 1, edSigner(adiKey, 2), wd)
			require.NoError(t, err)
			send(tx)
		})

		// Without the sleep, this test fails on Windows and macOS
		time.Sleep(3 * time.Second)

		// Test getting the data by URL
		rde := new(protocol.ResponseDataEntry)
		n.QueryAccountAs("FooBar/oof#data", rde)

		if !rde.Entry.Equal(&wd.Entry) {
			t.Fatalf("data query does not match what was entered")
		}

		//now test query by entry hash.
		rde2 := new(protocol.ResponseDataEntry)
		n.QueryAccountAs(fmt.Sprintf("FooBar/oof#data/%X", wd.Entry.Hash()), rde2)

		if !rde.Entry.Equal(&rde2.Entry) {
			t.Fatalf("data query does not match what was entered")
		}

		//now test query by entry set
		rde3 := new(protocol.ResponseDataEntrySet)
		n.QueryAccountAs("FooBar/oof#data/0:1", rde3)
		if !rde.Entry.Equal(&rde3.DataEntries[0].Entry) {
			t.Fatalf("data query does not match what was entered")
		}

	})
}

func TestCreateAdiTokenAccount(t *testing.T) {
	t.Run("Default Key Book", func(t *testing.T) {
		subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
		nodes := RunTestNet(t, subnets, daemons, nil, true)
		n := nodes[subnets[1]][0]

		adiKey := generateKey()
		batch := n.db.Begin()
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		require.NoError(t, batch.Commit())

		n.Batch(func(send func(*transactions.Envelope)) {
			tac := new(protocol.CreateTokenAccount)
			tac.Url = n.ParseUrl("FooBar/Baz")
			tac.TokenUrl = protocol.AcmeUrl()
			tx, err := transactions.New("FooBar", 1, edSigner(adiKey, 1), tac)
			require.NoError(t, err)
			send(tx)
		})

		r := n.GetTokenAccount("FooBar/Baz")
		require.Equal(t, "acc://FooBar/Baz", r.Url.String())
		require.Equal(t, protocol.AcmeUrl().String(), r.TokenUrl.String())

		require.Equal(t, []string{
			n.ParseUrl("FooBar").String(),
			n.ParseUrl("FooBar/book0").String(),
			n.ParseUrl("FooBar/page0").String(),
			n.ParseUrl("FooBar/Baz").String(),
		}, n.GetDirectory("FooBar"))
	})

	t.Run("Custom Key Book", func(t *testing.T) {
		subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
		nodes := RunTestNet(t, subnets, daemons, nil, true)
		n := nodes[subnets[1]][0]

		adiKey, pageKey := generateKey(), generateKey()
		batch := n.db.Begin()
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		require.NoError(t, acctesting.CreateKeyPage(batch, "foo/page1", pageKey.PubKey().Bytes()))
		require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", "foo/page1"))
		require.NoError(t, batch.Commit())

		n.Batch(func(send func(*transactions.Envelope)) {
			tac := new(protocol.CreateTokenAccount)
			tac.Url = n.ParseUrl("FooBar/Baz")
			tac.TokenUrl = protocol.AcmeUrl()
			tac.KeyBookUrl = n.ParseUrl("foo/book1")
			tx, err := transactions.New("FooBar", 1, edSigner(adiKey, 1), tac)
			require.NoError(t, err)
			send(tx)
		})

		u := n.ParseUrl("foo/book1")

		r := n.GetTokenAccount("FooBar/Baz")
		require.Equal(t, "acc://FooBar/Baz", r.Url.String())
		require.Equal(t, protocol.AcmeUrl().String(), r.TokenUrl.String())
		require.Equal(t, u.String(), r.KeyBook.String())
	})
}

func TestLiteAccountTx(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	alice, bob, charlie := generateKey(), generateKey(), generateKey()
	batch := n.db.Begin()
	require.NoError(n.t, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, 5e4, 1e9))
	require.NoError(n.t, acctesting.CreateLiteTokenAccount(batch, bob, 0))
	require.NoError(n.t, acctesting.CreateLiteTokenAccount(batch, charlie, 0))
	require.NoError(t, batch.Commit())

	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice).String()
	bobUrl := acctesting.AcmeLiteAddressTmPriv(bob).String()
	charlieUrl := acctesting.AcmeLiteAddressTmPriv(charlie).String()

	n.Batch(func(send func(*transactions.Envelope)) {
		exch := new(protocol.SendTokens)
		exch.AddRecipient(acctesting.MustParseUrl(bobUrl), big.NewInt(int64(1000)))
		exch.AddRecipient(acctesting.MustParseUrl(charlieUrl), big.NewInt(int64(2000)))

		tx, err := transactions.New(aliceUrl, 2, edSigner(alice, 1), exch)
		require.NoError(t, err)
		send(tx)
	})

	require.Equal(t, int64(5e4*acctesting.TokenMx-3000), n.GetLiteTokenAccount(aliceUrl).Balance.Int64())
	require.Equal(t, int64(1000), n.GetLiteTokenAccount(bobUrl).Balance.Int64())
	require.Equal(t, int64(2000), n.GetLiteTokenAccount(charlieUrl).Balance.Int64())
}

func TestAdiAccountTx(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	fooKey, barKey := generateKey(), generateKey()
	batch := n.db.Begin()
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, acctesting.CreateTokenAccount(batch, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
	require.NoError(t, acctesting.CreateADI(batch, barKey, "bar"))
	require.NoError(t, acctesting.CreateTokenAccount(batch, "bar/tokens", protocol.AcmeUrl().String(), 0, false))
	require.NoError(t, batch.Commit())

	n.Batch(func(send func(*transactions.Envelope)) {
		exch := new(protocol.SendTokens)
		exch.AddRecipient(n.ParseUrl("bar/tokens"), big.NewInt(int64(68)))

		tx, err := transactions.New("foo/tokens", 1, edSigner(fooKey, 1), exch)
		require.NoError(t, err)
		send(tx)
	})

	require.Equal(t, int64(acctesting.TokenMx-68), n.GetTokenAccount("foo/tokens").Balance.Int64())
	require.Equal(t, int64(68), n.GetTokenAccount("bar/tokens").Balance.Int64())
}

func TestSendCreditsFromAdiAccountToMultiSig(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	fooKey := generateKey()
	batch := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateTokenAccount(batch, "foo/tokens", protocol.AcmeUrl().String(), 1e2, false))
	require.NoError(t, batch.Commit())

	n.Batch(func(send func(*transactions.Envelope)) {
		ac := new(protocol.AddCredits)
		ac.Amount = 55
		ac.Recipient = n.ParseUrl("foo/page0")

		tx, err := transactions.New("foo/tokens", 1, edSigner(fooKey, 1), ac)
		require.NoError(t, err)
		send(tx)
	})

	ks := n.GetKeyPage("foo/page0")
	acct := n.GetTokenAccount("foo/tokens")
	require.Equal(t, int64(55), ks.CreditBalance.Int64())
	require.Equal(t, int64(protocol.AcmePrecision*1e2-protocol.AcmePrecision/protocol.CreditsPerFiatUnit*55), acct.Balance.Int64())
}

func TestCreateKeyPage(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	fooKey, testKey := generateKey(), generateKey()
	batch := n.db.Begin()
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, batch.Commit())

	n.Batch(func(send func(*transactions.Envelope)) {
		cms := new(protocol.CreateKeyPage)
		cms.Url = n.ParseUrl("foo/keyset1")
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
	require.Nil(t, spec.KeyBook)
	require.Equal(t, uint64(0), key.Nonce)
	require.Equal(t, testKey.PubKey().Bytes(), key.PublicKey)
}

func TestCreateKeyBook(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	fooKey, testKey := generateKey(), generateKey()
	batch := n.db.Begin()
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, acctesting.CreateKeyPage(batch, "foo/page1", testKey.PubKey().Bytes()))
	require.NoError(t, batch.Commit())

	specUrl := n.ParseUrl("foo/page1")

	groupUrl := n.ParseUrl("foo/book1")

	n.Batch(func(send func(*transactions.Envelope)) {
		csg := new(protocol.CreateKeyBook)
		csg.Url = n.ParseUrl("foo/book1")
		csg.Pages = append(csg.Pages, specUrl)

		tx, err := transactions.New("foo", 1, edSigner(fooKey, 1), csg)
		require.NoError(t, err)
		send(tx)
	})

	group := n.GetKeyBook("foo/book1")
	require.Len(t, group.Pages, 1)
	require.Equal(t, specUrl.String(), group.Pages[0].String())

	spec := n.GetKeyPage("foo/page1")
	require.Equal(t, spec.KeyBook.String(), groupUrl.String())
}

func TestAddKeyPage(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	fooKey, testKey1, testKey2 := generateKey(), generateKey(), generateKey()

	u := n.ParseUrl("foo/book1")

	batch := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyPage(batch, "foo/page1", testKey1.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", "foo/page1"))
	require.NoError(t, acctesting.AddCredits(batch, n.ParseUrl("foo/page1"), 1e9))
	require.NoError(t, batch.Commit())

	// Sanity check
	require.Equal(t, u.String(), n.GetKeyPage("foo/page1").KeyBook.String())

	n.Batch(func(send func(*transactions.Envelope)) {
		cms := new(protocol.CreateKeyPage)
		cms.Url = n.ParseUrl("foo/page2")
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			PublicKey: testKey2.PubKey().Bytes(),
		})

		tx, err := transactions.New("foo/book1", 1, edSigner(testKey1, 1), cms)
		require.NoError(t, err)
		send(tx)
	})

	spec := n.GetKeyPage("foo/page2")
	require.Len(t, spec.Keys, 1)
	key := spec.Keys[0]
	require.Equal(t, u.String(), spec.KeyBook.String())
	require.Equal(t, uint64(0), key.Nonce)
	require.Equal(t, testKey2.PubKey().Bytes(), key.PublicKey)
}

func TestAddKey(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	fooKey, testKey := generateKey(), generateKey()

	batch := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyPage(batch, "foo/page1", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", "foo/page1"))
	require.NoError(t, acctesting.AddCredits(batch, n.ParseUrl("foo/page1"), 1e9))
	require.NoError(t, batch.Commit())

	newKey := generateKey()
	n.Batch(func(send func(*transactions.Envelope)) {
		body := new(protocol.UpdateKeyPage)
		body.Operation = protocol.KeyPageOperationAdd
		body.NewKey = newKey.PubKey().Bytes()

		tx, err := transactions.New("foo/page1", 1, edSigner(testKey, 1), body)
		require.NoError(t, err)
		send(tx)
	})

	spec := n.GetKeyPage("foo/page1")
	require.Len(t, spec.Keys, 2)
	require.Equal(t, newKey.PubKey().Bytes(), spec.Keys[1].PublicKey)
}

func TestUpdateKey(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	fooKey, testKey := generateKey(), generateKey()

	batch := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyPage(batch, "foo/page1", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", "foo/page1"))
	require.NoError(t, acctesting.AddCredits(batch, n.ParseUrl("foo/page1"), 1e9))
	require.NoError(t, batch.Commit())

	newKey := generateKey()
	n.Batch(func(send func(*transactions.Envelope)) {
		body := new(protocol.UpdateKeyPage)
		body.Operation = protocol.KeyPageOperationUpdate
		body.Key = testKey.PubKey().Bytes()
		body.NewKey = newKey.PubKey().Bytes()

		tx, err := transactions.New("foo/page1", 1, edSigner(testKey, 1), body)
		require.NoError(t, err)
		send(tx)
	})

	spec := n.GetKeyPage("foo/page1")
	require.Len(t, spec.Keys, 1)
	require.Equal(t, newKey.PubKey().Bytes(), spec.Keys[0].PublicKey)
}

func TestRemoveKey(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	fooKey, testKey1, testKey2 := generateKey(), generateKey(), generateKey()

	batch := n.db.Begin()
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyPage(batch, "foo/page1", testKey1.PubKey().Bytes(), testKey2.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", "foo/page1"))
	require.NoError(t, acctesting.AddCredits(batch, n.ParseUrl("foo/page1"), 1e9))
	require.NoError(t, batch.Commit())

	n.Batch(func(send func(*transactions.Envelope)) {
		body := new(protocol.UpdateKeyPage)
		body.Operation = protocol.KeyPageOperationRemove
		body.Key = testKey1.PubKey().Bytes()

		tx, err := transactions.New("foo/page1", 1, edSigner(testKey2, 1), body)
		require.NoError(t, err)
		send(tx)
	})

	spec := n.GetKeyPage("foo/page1")
	require.Len(t, spec.Keys, 1)
	require.Equal(t, testKey2.PubKey().Bytes(), spec.Keys[0].PublicKey)
}

func TestSignatorHeight(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	liteKey, fooKey := generateKey(), generateKey()

	liteUrl, err := protocol.LiteTokenAddress(liteKey.PubKey().Bytes(), "ACME")
	require.NoError(t, err)
	tokenUrl, err := url.Parse("foo/tokens")
	require.NoError(t, err)
	keyPageUrl, err := url.Parse("foo/page0")
	require.NoError(t, err)

	batch := n.db.Begin()
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteKey, 1, 1e9))
	require.NoError(t, batch.Commit())

	getHeight := func(u *url.URL) uint64 {
		batch := n.db.Begin()
		defer batch.Discard()
		chain, err := batch.Account(u).ReadChain(protocol.MainChain)
		require.NoError(t, err)
		return uint64(chain.Height())
	}

	n.Batch(func(send func(*transactions.Envelope)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = n.ParseUrl("foo")
		adi.PublicKey = fooKey.PubKey().Bytes()
		adi.KeyBookName = "book"
		adi.KeyPageName = "page0"

		tx, err := transactions.New(liteUrl.String(), 1, edSigner(liteKey, 1), adi)
		require.NoError(t, err)
		send(tx)
	})

	batch = n.db.Begin()
	require.NoError(t, acctesting.AddCredits(batch, n.ParseUrl("foo/page0"), 1e9))
	require.NoError(t, batch.Commit())

	keyPageHeight := getHeight(keyPageUrl)

	n.Batch(func(send func(*transactions.Envelope)) {
		tac := new(protocol.CreateTokenAccount)
		tac.Url = tokenUrl
		tac.TokenUrl = protocol.AcmeUrl()
		tx, err := transactions.New("foo", 1, edSigner(fooKey, 1), tac)
		require.NoError(t, err)
		send(tx)
	})

	require.Equal(t, keyPageHeight, getHeight(keyPageUrl), "Key page height changed")
}

func TestCreateToken(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	fooKey := generateKey()
	batch := n.db.Begin()
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, batch.Commit())

	n.Batch(func(send func(*transactions.Envelope)) {
		body := new(protocol.CreateToken)
		body.Url = n.ParseUrl("foo/tokens")
		body.Symbol = "FOO"
		body.Precision = 10

		tx, err := transactions.New("foo", 1, edSigner(fooKey, 1), body)
		require.NoError(t, err)
		send(tx)
	})

	n.GetTokenIssuer("foo/tokens")
}

func TestIssueTokens(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	fooKey, liteKey := generateKey(), generateKey()
	batch := n.db.Begin()
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, acctesting.CreateTokenIssuer(batch, "foo/tokens", "FOO", 10))
	require.NoError(t, batch.Commit())

	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], "foo/tokens")
	require.NoError(t, err)

	n.Batch(func(send func(*transactions.Envelope)) {
		body := new(protocol.IssueTokens)
		body.Recipient = liteAddr
		body.Amount.SetUint64(123)

		tx, err := transactions.New("foo/tokens", 1, edSigner(fooKey, 1), body)
		require.NoError(t, err)
		send(tx)
	})

	account := n.GetLiteTokenAccount(liteAddr.String())
	require.Equal(t, "acc://foo/tokens", account.TokenUrl.String())
	require.Equal(t, int64(123), account.Balance.Int64())
}

func TestInvalidDeposit(t *testing.T) {
	// The lite address ends with `foo/tokens` but the token is `foo2/tokens` so
	// the synthetic transaction will fail. This test verifies that the
	// transaction fails, but more importantly it verifies that
	// `Executor.Commit()` does *not* break if DeliverTx fails with a
	// non-existent origin. This is motivated by a bug that has been fixed. This
	// bug could have been triggered by a failing SyntheticCreateChains,
	// SyntheticDepositTokens, or SyntheticDepositCredits.

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	liteKey := generateKey()
	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], "foo/tokens")
	require.NoError(t, err)

	id := n.Batch(func(send func(*transactions.Envelope)) {
		body := new(protocol.SyntheticDepositTokens)
		body.Token = n.ParseUrl("foo2/tokens")
		body.Amount.SetUint64(123)

		tx, err := transactions.New(liteAddr.String(), 1, edSigner(n.key.Bytes(), 1), body)
		require.NoError(t, err)
		send(tx)
	})[0]

	tx := n.GetTx(id[:])
	require.NotZero(t, tx.Status.Code)
}

func DumpAccount(t *testing.T, batch *database.Batch, accountUrl *url.URL) {
	account := batch.Account(accountUrl)
	state, err := account.GetState()
	require.NoError(t, err)
	fmt.Println("Dump", accountUrl, state.GetType())
	meta, err := account.GetObject()
	require.NoError(t, err)
	seen := map[[32]byte]bool{}
	for _, cmeta := range meta.Chains {
		chain, err := account.ReadChain(cmeta.Name)
		require.NoError(t, err)
		fmt.Printf("  Chain: %s (%v)\n", cmeta.Name, cmeta.Type)
		height := chain.Height()
		entries, err := chain.Entries(0, height)
		require.NoError(t, err)
		for idx, id := range entries {
			fmt.Printf("    Entry %d: %X\n", idx, id)
			if cmeta.Type != protocol.ChainTypeTransaction {
				continue
			}
			var id32 [32]byte
			require.Equal(t, 32, copy(id32[:], id))
			if seen[id32] {
				continue
			}
			txState, txStatus, txSigs, err := batch.Transaction(id32[:]).Get()
			require.NoError(t, err)
			if seen[*txState.TransactionHash()] {
				fmt.Printf("      TX: hash=%X\n", *txState.TransactionHash())
				continue
			}
			fmt.Printf("      TX: type=%v origin=%v status=%#v sigs=%d\n", txState.TxType(), txState.Url, txStatus, len(txSigs))
			seen[id32] = true
		}
	}
}
