package abci_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	types2 "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/testing/e2e"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
	randpkg "golang.org/x/exp/rand"
)

var rand = randpkg.New(randpkg.NewSource(0))

type Tx = protocol.Envelope

func init() { acctesting.EnableDebugFeatures() }

func TestEndToEndSuite(t *testing.T) {
	t.Skip("This is failing and may be more trouble than it's worth")

	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) e2e.DUT {
		// Recreate the app for each test
		subnets, daemons := acctesting.CreateTestNet(s.T(), 1, 1, 0)
		nodes := RunTestNet(s.T(), subnets, daemons, nil, true, nil)
		n := nodes[subnets[1]][0]

		return &e2eDUT{s, n}
	}))
}

func TestCreateLiteAccount(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	const N, M = 11, 1
	const count = N * M
	credits := 10000.0
	originAddr, balances := n.testLiteTx(N, M, credits)
	amountSent := float64(count * 1000)
	initialAmount := protocol.AcmeFaucetAmount * protocol.AcmePrecision
	currentBalance := n.GetLiteTokenAccount(originAddr).Balance.Int64()
	totalAmountSent := initialAmount - amountSent
	require.Equal(t, int64(totalAmountSent), currentBalance)
	for addr, bal := range balances {
		require.Equal(t, bal, n.GetLiteTokenAccount(addr).Balance.Int64())
	}
}

func TestEvilNode(t *testing.T) {

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	//tell the TestNet that we have an evil node in the midst
	dns := subnets[0]
	bvn := subnets[1]
	subnets[0] = "evil-" + subnets[0]
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)

	dn := nodes[dns][0]
	n := nodes[bvn][0]

	var count = 11
	credits := 100.0
	originAddr, balances := n.testLiteTx(count, 1, credits)
	require.Equal(t, int64(protocol.AcmeFaucetAmount*protocol.AcmePrecision-count*1000), n.GetLiteTokenAccount(originAddr).Balance.Int64())
	for addr, bal := range balances {
		require.Equal(t, bal, n.GetLiteTokenAccount(addr).Balance.Int64())
	}

	batch := dn.db.Begin(true)
	defer batch.Discard()
	evData, err := batch.Account(dn.network.NodeUrl(protocol.Evidence)).Data()
	require.NoError(t, err)
	// Check each anchor
	_, de, err := evData.GetLatest()
	require.NoError(t, err)
	var ev []types2.Evidence
	require.NotEqual(t, de.Data, nil, "no data")
	err = json.Unmarshal(de.Data[0], &ev)
	require.NoError(t, err)
	require.Greaterf(t, len(ev), 0, "no evidence data")
	require.Greater(t, ev[0].Height, int64(0), "no valid evidence available")

}

func (n *FakeNode) testLiteTx(N, M int, credits float64) (string, map[string]int64) {
	sender := generateKey()
	senderUrl := acctesting.AcmeLiteAddressTmPriv(sender)

	recipients := make([]string, N)
	for i := range recipients {
		_, key, _ := ed25519.GenerateKey(nil)
		recipients[i] = acctesting.AcmeLiteAddressStdPriv(key).String()
	}

	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.AcmeFaucet)
		body.Url = senderUrl

		send(acctesting.NewTransaction().
			WithPrincipal(protocol.FaucetUrl.RootIdentity()).
			WithBody(body).
			Faucet())
	})

	batch := n.db.Begin(true)
	//acme to credits @ $0.05 acme price is 1:5

	liteTokenId := senderUrl.RootIdentity()
	n.Require().NoError(acctesting.AddCredits(batch, liteTokenId, credits))
	n.require.NoError(batch.Commit())

	balance := map[string]int64{}
	for i := 0; i < M; i++ {
		n.MustExecuteAndWait(func(send func(*Tx)) {
			for i := 0; i < N; i++ {
				recipient := recipients[rand.Intn(len(recipients))]
				balance[recipient] += 1000

				exch := new(protocol.SendTokens)
				exch.AddRecipient(n.ParseUrl(recipient), big.NewInt(int64(1000)))
				send(newTxn(senderUrl.String()).
					WithBody(exch).
					Initiate(protocol.SignatureTypeLegacyED25519, sender).
					Build())
			}
		})
	}

	return senderUrl.String(), balance
}

func TestFaucet(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	alice := generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)

	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.AcmeFaucet)
		body.Url = aliceUrl

		faucet := protocol.Faucet.Signer()
		send(acctesting.NewTransaction().
			WithPrincipal(protocol.FaucetUrl).
			WithTimestamp(faucet.Timestamp()).
			WithBody(body).
			Faucet())
	})

	require.Equal(t, int64(protocol.AcmeFaucetAmount*protocol.AcmePrecision), n.GetLiteTokenAccount(aliceUrl.String()).Balance.Int64())
}

func TestAnchorChain(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]
	dn := nodes[subnets[0]][0]

	liteAccount := generateKey()
	newAdi := generateKey()
	keyHash := sha256.Sum256(newAdi.PubKey().Address())

	batch := n.db.Begin(true)
	require.NoError(n.t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteAccount, protocol.AcmeFaucetAmount, 1e6))
	require.NoError(t, batch.Commit())

	n.MustExecuteAndWait(func(send func(*Tx)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = n.ParseUrl("RoadRunner")
		var err error
		adi.KeyBookUrl, err = url.Parse(fmt.Sprintf("%s/book", adi.Url))
		require.NoError(t, err)
		adi.KeyHash = keyHash[:]

		sponsorUrl := acctesting.AcmeLiteAddressTmPriv(liteAccount).RootIdentity().String()
		send(newTxn(sponsorUrl).
			WithBody(adi).
			Initiate(protocol.SignatureTypeLegacyED25519, liteAccount).
			Build())
	})

	// Sanity check
	require.Equal(t, "acc://RoadRunner", n.GetADI("RoadRunner").Url.String())

	// // Get the anchor chain manager
	// batch = n.db.Begin(true)
	// defer batch.Discard()
	// ledger := batch.Account(n.network.NodeUrl(protocol.Ledger))

	// // Check each anchor
	// // TODO FIX This is broken because the ledger no longer has a list of updates
	// var ledgerState *protocol.InternalLedger
	// require.NoError(t, ledger.GetStateAs(&ledgerState))
	// rootChain, err := ledger.ReadChain(protocol.MinorRootChain)
	// require.NoError(t, err)
	// first := rootChain.Height() - int64(len(ledgerState.Updates))
	// for i, meta := range ledgerState.Updates {
	// 	root, err := rootChain.Entry(first + int64(i))
	// 	require.NoError(t, err)

	// 	if meta.Name == "bpt" {
	// 		assert.Equal(t, root, batch.BptRootHash(), "wrong anchor for BPT")
	// 		continue
	// 	}

	// 	mgr, err := batch.Account(meta.Account).ReadChain(meta.Name)
	// 	require.NoError(t, err)

	// 	assert.Equal(t, root, mgr.Anchor(), "wrong anchor for %s#chain/%s", meta.Account, meta.Name)
	// }

	//set price of acme to $445.00 / token
	price := 445.00
	dn.MustExecuteAndWait(func(send func(*Tx)) {
		ao := new(protocol.AcmeOracle)
		ao.Price = uint64(price * protocol.AcmeOraclePrecision)
		wd := new(protocol.WriteData)
		d, err := json.Marshal(&ao)
		require.NoError(t, err)
		wd.Entry.Data = append(wd.Entry.Data, d)

		originUrl := protocol.PriceOracleAuthority

		send(newTxn(originUrl).
			WithSigner(dn.network.ValidatorPage(0), 1).
			WithBody(wd).
			Initiate(protocol.SignatureTypeLegacyED25519, dn.key.Bytes()).
			Build())
	})

	// Give it a second for the DN to send its anchor
	time.Sleep(time.Second)

	// Get the anchor chain manager for DN
	batch = dn.db.Begin(true)
	defer batch.Discard()
	ledger := batch.Account(dn.network.NodeUrl(protocol.Ledger))
	// Check each anchor
	var ledgerState *protocol.InternalLedger
	require.NoError(t, ledger.GetStateAs(&ledgerState))
	expected := uint64(price * protocol.AcmeOraclePrecision)
	require.Equal(t, int(expected), int(ledgerState.ActiveOracle))

	time.Sleep(2 * time.Second)
	// Get the anchor chain manager for BVN
	batch = n.db.Begin(true)
	defer batch.Discard()
	ledger = batch.Account(n.network.NodeUrl(protocol.Ledger))

	// Check each anchor
	ledgerState = new(protocol.InternalLedger)
	require.NoError(t, ledger.GetStateAs(&ledgerState))
	require.Equal(t, ledgerState.ActiveOracle, expected)

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
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	liteAccount := generateKey()
	newAdi := generateKey()
	keyHash := sha256.Sum256(newAdi.PubKey().Address())
	batch := n.db.Begin(true)
	require.NoError(n.t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteAccount, protocol.AcmeFaucetAmount, 1e6))
	require.NoError(t, batch.Commit())

	n.MustExecuteAndWait(func(send func(*Tx)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = n.ParseUrl("RoadRunner")
		adi.KeyHash = keyHash[:]
		var err error
		adi.KeyBookUrl, err = url.Parse(fmt.Sprintf("%s/foo-book", adi.Url))
		require.NoError(t, err)

		sponsorUrl := acctesting.AcmeLiteAddressTmPriv(liteAccount).RootIdentity().String()
		send(newTxn(sponsorUrl).
			WithBody(adi).
			Initiate(protocol.SignatureTypeLegacyED25519, liteAccount).
			Build())
	})

	r := n.GetADI("RoadRunner")
	require.Equal(t, "acc://RoadRunner", r.Url.String())

	kg := n.GetKeyBook("RoadRunner/foo-book")
	require.Equal(t, uint64(1), kg.PageCount)

	ks := n.GetKeyPage("RoadRunner/foo-book/1")
	require.Len(t, ks.Keys, 1)
	require.Equal(t, keyHash[:], ks.Keys[0].PublicKeyHash)
}

func TestCreateADIWithoutKeybook(t *testing.T) {
	check := CheckError{H: NewDefaultErrorHandler(t), Disable: true}

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, check.ErrorHandler())
	n := nodes[subnets[1]][0]

	liteAccount := generateKey()
	newAdi := generateKey()
	keyHash := sha256.Sum256(newAdi.PubKey().Address())
	batch := n.db.Begin(true)
	require.NoError(n.t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteAccount, protocol.AcmeFaucetAmount, 1e6))
	require.NoError(t, batch.Commit())

	_, _, err := n.Execute(func(send func(*Tx)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = n.ParseUrl("RoadRunner")
		adi.KeyHash = keyHash[:]

		sponsorUrl := acctesting.AcmeLiteAddressTmPriv(liteAccount).String()
		send(newTxn(sponsorUrl).
			WithBody(adi).
			Initiate(protocol.SignatureTypeLegacyED25519, liteAccount).
			Build())
	})
	require.Error(t, err)
}

func TestCreateLiteDataAccount(t *testing.T) {

	//this test exercises WriteDataTo and SyntheticWriteData validators

	firstEntry := protocol.DataEntry{}

	firstEntry.Data = append(firstEntry.Data, []byte{})
	firstEntry.Data = append(firstEntry.Data, []byte("Factom PRO"))
	firstEntry.Data = append(firstEntry.Data, []byte("Tutorial"))

	//create a lite data account aka factom chainId
	chainId := protocol.ComputeLiteDataAccountId(&firstEntry)

	liteDataAddress, err := protocol.LiteDataAddress(chainId)
	if err != nil {
		t.Fatal(err)
	}

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	adiKey := generateKey()
	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
	require.NoError(t, batch.Commit())
	ids := n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		wdt := new(protocol.WriteDataTo)
		wdt.Recipient = liteDataAddress
		wdt.Entry = firstEntry
		send(newTxn("FooBar").
			WithSigner(url.MustParse("FooBar/book0/1"), 1).
			WithBody(wdt).
			Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
			Build())
	})

	partialChainId, err := protocol.ParseLiteDataAddress(liteDataAddress)
	if err != nil {
		t.Fatal(err)
	}
	r := n.GetLiteDataAccount(liteDataAddress.String())
	require.Equal(t, liteDataAddress.String(), r.Url.String())
	require.Equal(t, append(partialChainId, r.Tail...), chainId)

	firstEntryHash, err := protocol.ComputeLiteEntryHashFromEntry(chainId, &firstEntry)
	require.NoError(t, err)

	batch = n.db.Begin(false)
	defer batch.Discard()

	synthIds, err := batch.Transaction(ids[0][:]).GetSyntheticTxns()
	require.NoError(t, err)

	// Verify the entry hash in the transaction result
	txStatus, err := batch.Transaction(synthIds.Hashes[0][:]).GetStatus()
	require.NoError(t, err)
	require.IsType(t, (*protocol.WriteDataResult)(nil), txStatus.Result)
	txResult := txStatus.Result.(*protocol.WriteDataResult)
	require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(txResult.EntryHash[:]), "Transaction result entry hash does not match")

	// Verify the entry hash returned by Entry
	dataChain, err := batch.Account(liteDataAddress).Data()
	require.NoError(t, err)
	entry, err := dataChain.Entry(0)
	require.NoError(t, err)
	hashFromEntry, err := protocol.ComputeLiteEntryHashFromEntry(chainId, entry)
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(hashFromEntry), "Chain Entry.Hash does not match")
	//sample verification for calculating the hash from lite data entry
	hashes, err := dataChain.GetHashes(0, 1)
	require.NoError(t, err)
	ent, err := dataChain.Entry(0)
	require.NoError(t, err)
	id := protocol.ComputeLiteDataAccountId(ent)
	newh, err := protocol.ComputeLiteEntryHashFromEntry(id, ent)
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(hashes[0]), "Chain GetHashes does not match")
	require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(newh), "Chain GetHashes does not match")

}

func TestCreateAdiDataAccount(t *testing.T) {

	t.Run("Data Account w/ Default Key Book and no Manager Key Book", func(t *testing.T) {
		subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
		nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
		n := nodes[subnets[1]][0]

		adiKey := generateKey()
		batch := n.db.Begin(true)
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		require.NoError(t, batch.Commit())

		n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
			tac := new(protocol.CreateDataAccount)
			tac.Url = n.ParseUrl("FooBar/oof")
			send(newTxn("FooBar").
				WithSigner(url.MustParse("FooBar/book0/1"), 1).
				WithBody(tac).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				Build())
		})

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, "acc://FooBar/oof", r.Url.String())

		require.Contains(t, n.GetDirectory("FooBar"), n.ParseUrl("FooBar/oof").String())
	})

	t.Run("Data Account w/ Custom Key Book and Manager Key Book Url", func(t *testing.T) {
		subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
		nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
		n := nodes[subnets[1]][0]

		adiKey, pageKey := generateKey(), generateKey()
		batch := n.db.Begin(true)
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		require.NoError(t, acctesting.CreateKeyBook(batch, "acc://FooBar/foo/book1", pageKey.PubKey().Bytes()))
		require.NoError(t, acctesting.CreateKeyPage(batch, "acc://FooBar/foo/book1"))
		require.NoError(t, acctesting.CreateKeyBook(batch, "acc://FooBar/mgr/book1", nil))
		require.NoError(t, acctesting.CreateKeyPage(batch, "acc://FooBar/mgr/book1", pageKey.PubKey().Bytes()))
		require.NoError(t, batch.Commit())

		n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
			cda := new(protocol.CreateDataAccount)
			cda.Url = n.ParseUrl("FooBar/oof")
			cda.KeyBookUrl = n.ParseUrl("acc://FooBar/foo/book1")
			cda.ManagerKeyBookUrl = n.ParseUrl("acc://FooBar/mgr/book1")
			send(newTxn("FooBar").
				WithSigner(url.MustParse("FooBar/book0/1"), 1).
				WithBody(cda).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				Build())
		})

		u := n.ParseUrl("acc://FooBar/foo/book1")

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, "acc://FooBar/oof", r.Url.String())
		require.Equal(t, "acc://FooBar/mgr/book1", r.ManagerKeyBook().String())
		require.Equal(t, u.String(), r.KeyBook().String())

	})

	t.Run("Data Account data entry", func(t *testing.T) {
		subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
		nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
		n := nodes[subnets[1]][0]

		adiKey := generateKey()
		batch := n.db.Begin(true)
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		require.NoError(t, batch.Commit())

		n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
			tac := new(protocol.CreateDataAccount)
			tac.Url = n.ParseUrl("FooBar/oof")
			send(newTxn("FooBar").
				WithSigner(url.MustParse("FooBar/book0/1"), 1).
				WithBody(tac).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				Build())
		})

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, "acc://FooBar/oof", r.Url.String())
		require.Contains(t, n.GetDirectory("FooBar"), n.ParseUrl("FooBar/oof").String())

		wd := new(protocol.WriteData)
		n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
			wd.Entry.Data = append(wd.Entry.Data, []byte("thequickbrownfoxjumpsoverthelazydog"))
			for i := 0; i < 10; i++ {
				wd.Entry.Data = append(wd.Entry.Data, []byte(fmt.Sprintf("test id %d", i)))
			}

			send(newTxn("FooBar/oof").
				WithSigner(url.MustParse("FooBar/book0/1"), 1).
				WithBody(wd).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				Build())
		})

		// Without the sleep, this test fails on Windows and macOS
		time.Sleep(3 * time.Second)

		// Test getting the data by URL
		rde := new(query.ResponseDataEntry)
		n.QueryAccountAs("FooBar/oof#data", rde)

		if !rde.Entry.Equal(&wd.Entry) {
			t.Fatalf("data query does not match what was entered")
		}

		//now test query by entry hash.
		rde2 := new(query.ResponseDataEntry)
		n.QueryAccountAs(fmt.Sprintf("FooBar/oof#data/%X", wd.Entry.Hash()), rde2)

		if !rde.Entry.Equal(&rde2.Entry) {
			t.Fatalf("data query does not match what was entered")
		}

		//now test query by entry set
		rde3 := new(query.ResponseDataEntrySet)
		n.QueryAccountAs("FooBar/oof#data/0:1", rde3)
		if !rde.Entry.Equal(&rde3.DataEntries[0].Entry) {
			t.Fatalf("data query does not match what was entered")
		}

	})
}

func TestCreateAdiTokenAccount(t *testing.T) {
	t.Run("Default Key Book", func(t *testing.T) {
		subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
		nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
		n := nodes[subnets[1]][0]

		adiKey := generateKey()
		batch := n.db.Begin(true)
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		require.NoError(t, batch.Commit())

		n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
			tac := new(protocol.CreateTokenAccount)
			tac.Url = n.ParseUrl("FooBar/Baz")
			tac.TokenUrl = protocol.AcmeUrl()
			send(newTxn("FooBar").
				WithSigner(url.MustParse("FooBar/book0/1"), 1).
				WithBody(tac).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				Build())
		})

		r := n.GetTokenAccount("FooBar/Baz")
		require.Equal(t, "acc://FooBar/Baz", r.Url.String())
		require.Equal(t, protocol.AcmeUrl().String(), r.TokenUrl.String())

		require.Equal(t, []string{
			n.ParseUrl("FooBar/book0").String(),
			n.ParseUrl("FooBar/book0/1").String(),
			n.ParseUrl("FooBar/Baz").String(),
		}, n.GetDirectory("FooBar"))
	})

	t.Run("Custom Key Book", func(t *testing.T) {
		subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
		nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
		n := nodes[subnets[1]][0]

		adiKey, pageKey := generateKey(), generateKey()
		batch := n.db.Begin(true)
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", pageKey.PubKey().Bytes()))
		require.NoError(t, batch.Commit())

		n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
			tac := new(protocol.CreateTokenAccount)
			tac.Url = n.ParseUrl("FooBar/Baz")
			tac.TokenUrl = protocol.AcmeUrl()
			tac.KeyBookUrl = n.ParseUrl("foo/book1")
			send(newTxn("FooBar").
				WithSigner(url.MustParse("FooBar/book0/1"), 1).
				WithBody(tac).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				Build())
		})

		u := n.ParseUrl("foo/book1")

		r := n.GetTokenAccount("FooBar/Baz")
		require.Equal(t, "acc://FooBar/Baz", r.Url.String())
		require.Equal(t, protocol.AcmeUrl().String(), r.TokenUrl.String())
		require.Equal(t, u.String(), r.KeyBook().String())
	})
}

func TestLiteAccountTx(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	alice, bob, charlie := generateKey(), generateKey(), generateKey()
	batch := n.db.Begin(true)
	require.NoError(n.t, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, protocol.AcmeFaucetAmount, 1e9))
	require.NoError(n.t, acctesting.CreateLiteTokenAccount(batch, bob, 0))
	require.NoError(n.t, acctesting.CreateLiteTokenAccount(batch, charlie, 0))
	require.NoError(t, batch.Commit())

	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	bobUrl := acctesting.AcmeLiteAddressTmPriv(bob).String()
	charlieUrl := acctesting.AcmeLiteAddressTmPriv(charlie).String()

	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		exch := new(protocol.SendTokens)
		exch.AddRecipient(acctesting.MustParseUrl(bobUrl), big.NewInt(int64(1000)))
		exch.AddRecipient(acctesting.MustParseUrl(charlieUrl), big.NewInt(int64(2000)))

		send(newTxn(aliceUrl.String()).
			WithSigner(aliceUrl.RootIdentity(), 1).
			WithBody(exch).
			Initiate(protocol.SignatureTypeLegacyED25519, alice).
			Build())
	})

	require.Equal(t, int64(protocol.AcmeFaucetAmount*protocol.AcmePrecision-3000), n.GetLiteTokenAccount(aliceUrl.String()).Balance.Int64())
	require.Equal(t, int64(1000), n.GetLiteTokenAccount(bobUrl).Balance.Int64())
	require.Equal(t, int64(2000), n.GetLiteTokenAccount(charlieUrl).Balance.Int64())
}

func TestAdiAccountTx(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	fooKey, barKey := generateKey(), generateKey()
	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, acctesting.CreateTokenAccount(batch, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
	require.NoError(t, acctesting.CreateADI(batch, barKey, "bar"))
	require.NoError(t, acctesting.CreateTokenAccount(batch, "bar/tokens", protocol.AcmeUrl().String(), 0, false))
	require.NoError(t, batch.Commit())

	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		exch := new(protocol.SendTokens)
		exch.AddRecipient(n.ParseUrl("bar/tokens"), big.NewInt(int64(68)))

		send(newTxn("foo/tokens").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(exch).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	require.Equal(t, int64(protocol.AcmePrecision-68), n.GetTokenAccount("foo/tokens").Balance.Int64())
	require.Equal(t, int64(68), n.GetTokenAccount("bar/tokens").Balance.Int64())
}

func TestSendTokensToBadRecipient(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	alice := generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	batch := n.db.Begin(true)
	require.NoError(n.t, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, protocol.AcmeFaucetAmount, 1e9))
	require.NoError(t, batch.Commit())

	// The send should succeed
	txnHashes := n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		exch := new(protocol.SendTokens)
		exch.AddRecipient(acctesting.MustParseUrl("foo"), big.NewInt(int64(1000)))

		send(newTxn(aliceUrl.String()).
			WithSigner(aliceUrl.RootIdentity(), 1).
			WithBody(exch).
			Initiate(protocol.SignatureTypeLegacyED25519, alice).
			Build())
	})

	// The synthetic transaction should fail
	res, err := n.api.QueryTx(txnHashes[0][:], time.Second, true, api.QueryOptions{})
	require.NoError(t, err)
	res, err = n.api.QueryTx(res.SyntheticTxids[0][:], time.Second, true, api.QueryOptions{})
	require.NoError(t, err)
	require.Equal(t, protocol.ErrorCodeNotFound.GetEnumValue(), res.Status.Code)

	// Give the synthetic receipt a second to resolve - workaround AC-1238
	time.Sleep(time.Second)

	// The balance should be unchanged
	require.Equal(t, int64(protocol.AcmeFaucetAmount*protocol.AcmePrecision), n.GetLiteTokenAccount(aliceUrl.String()).Balance.Int64())
}

func TestAddCreditsBurnAcme(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	fooKey := generateKey()
	batch := n.db.Begin(true)
	defer batch.Discard()
	acmeAmount := 100.00

	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateTokenAccount(batch, "foo/tokens", protocol.AcmeUrl().String(), acmeAmount, false))

	require.NoError(t, batch.Commit())

	require.NoError(t, nodes[subnets[0]][0].db.Update(func(batch *database.Batch) error {
		return acctesting.UpdateAccount(batch, protocol.AcmeUrl(), func(acme *protocol.TokenIssuer) {
			// Make it easier to read the value
			acme.Issued.SetUint64(1e3 * protocol.AcmePrecision)
		})
	}))

	acmeIssuer := n.GetTokenIssuer("acc://ACME")
	acmeBeforeBurn := acmeIssuer.Issued
	acmeToSpendOnCredits := int64(10.0 * protocol.AcmePrecision)
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		ac := new(protocol.AddCredits)
		ac.Amount = *big.NewInt(acmeToSpendOnCredits)
		ac.Recipient = n.ParseUrl("foo/book0/1")
		ac.Oracle = 500

		send(newTxn("foo/tokens").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(ac).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	batch = n.db.Begin(false)
	defer batch.Discard()
	ledger := batch.Account(n.network.NodeUrl(protocol.Ledger))

	// Check each anchor
	var ledgerState *protocol.InternalLedger
	require.NoError(t, ledger.GetStateAs(&ledgerState))
	//Credits I should have received
	credits := big.NewInt(protocol.CreditUnitsPerFiatUnit)                // want to obtain credits
	credits.Mul(credits, big.NewInt(int64(ledgerState.ActiveOracle)))     // fiat units / acme
	credits.Mul(credits, big.NewInt(acmeToSpendOnCredits))                // acme the user wants to spend
	credits.Div(credits, big.NewInt(int64(protocol.AcmeOraclePrecision))) // adjust the precision of oracle to real units
	credits.Div(credits, big.NewInt(int64(protocol.AcmePrecision)))       // adjust the precision of acme to spend to real units

	expectedCreditsToReceive := credits.Uint64()
	//the balance of the account should be

	ks := n.GetKeyPage("foo/book0/1")
	acct := n.GetTokenAccount("foo/tokens")
	acmeIssuer = n.GetTokenIssuer(protocol.AcmeUrl().String())
	acmeAfterBurn := acmeIssuer.Issued
	require.Equal(t, expectedCreditsToReceive, ks.CreditBalance)
	require.Equal(t, int64(acmeAmount*protocol.AcmePrecision)-acmeToSpendOnCredits, acct.Balance.Int64())
	require.Equal(t,
		protocol.FormatBigAmount(acmeBeforeBurn.Sub(&acmeBeforeBurn, big.NewInt(acmeToSpendOnCredits)), protocol.AcmePrecisionPower),
		protocol.FormatBigAmount(&acmeAfterBurn, protocol.AcmePrecisionPower))
}

func TestCreateKeyPage(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	fooKey, testKey := generateKey(), generateKey()
	fkh := sha256.Sum256(fooKey.PubKey().Bytes())
	tkh := sha256.Sum256(testKey.PubKey().Bytes())
	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, batch.Commit())

	page := n.GetKeyPage("foo/book0/1")
	require.Len(t, page.Keys, 1)
	key := page.Keys[0]
	require.Equal(t, uint64(0), key.LastUsedOn)
	require.Equal(t, fkh[:], key.PublicKeyHash)

	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		cms := new(protocol.CreateKeyPage)
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			KeyHash: tkh[:],
		})

		send(newTxn("foo/book0").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(cms).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	page = n.GetKeyPage("foo/book0/2")
	require.Len(t, page.Keys, 1)
	key = page.Keys[0]
	require.Equal(t, uint64(0), key.LastUsedOn)
	require.Equal(t, tkh[:], key.PublicKeyHash)
}

func TestCreateKeyBook(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	fooKey, testKey := generateKey(), generateKey()
	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, batch.Commit())

	bookUrl := n.ParseUrl("foo/book1")
	pageUrl := n.ParseUrl("foo/book1/1")

	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		csg := new(protocol.CreateKeyBook)
		csg.Url = n.ParseUrl("foo/book1")
		csg.PublicKeyHash = testKey.PubKey().Bytes()

		send(newTxn("foo").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(csg).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	book := n.GetKeyBook("foo/book1")
	require.Equal(t, uint64(1), book.PageCount)
	require.Equal(t, bookUrl, book.Url)

	page := n.GetKeyPage("foo/book1/1")
	require.Equal(t, pageUrl, page.Url)
}

func TestAddKeyPage(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	fooKey, testKey1, testKey2 := generateKey(), generateKey(), generateKey()

	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", testKey1.PubKey().Bytes()))
	require.NoError(t, acctesting.AddCredits(batch, n.ParseUrl("foo/book1/1"), 1e9))
	require.NoError(t, batch.Commit())

	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		cms := new(protocol.CreateKeyPage)
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			KeyHash: testKey2.PubKey().Bytes(),
		})

		send(newTxn("foo/book1").
			WithSigner(url.MustParse("foo/book1/1"), 1).
			WithBody(cms).
			Initiate(protocol.SignatureTypeLegacyED25519, testKey1).
			Build())
	})

	page := n.GetKeyPage("foo/book1/2")
	require.Len(t, page.Keys, 1)
	key := page.Keys[0]
	require.Equal(t, uint64(0), key.LastUsedOn)
	require.Equal(t, testKey2.PubKey().Bytes(), key.PublicKeyHash)
}

func TestAddKey(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	fooKey, testKey := generateKey(), generateKey()

	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.AddCredits(batch, n.ParseUrl("foo/book1/1"), 1e9))
	require.NoError(t, batch.Commit())

	newKey := generateKey()
	nkh := sha256.Sum256(newKey.PubKey().Bytes())

	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		op := new(protocol.AddKeyOperation)
		op.Entry.KeyHash = nkh[:]
		body := new(protocol.UpdateKeyPage)
		body.Operation = append(body.Operation, op)

		send(newTxn("foo/book1/1").
			WithSigner(url.MustParse("foo/book1/1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, testKey).
			Build())
	})

	page := n.GetKeyPage("foo/book1/1")
	require.Len(t, page.Keys, 2)
	require.Equal(t, nkh[:], page.Keys[1].PublicKeyHash)
}

func TestUpdateKeyPage(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	fooKey, testKey := generateKey(), generateKey()

	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.AddCredits(batch, n.ParseUrl("foo/book1/1"), 1e9))
	require.NoError(t, batch.Commit())

	newKey := generateKey()
	kh := sha256.Sum256(testKey.PubKey().Bytes())
	nkh := sha256.Sum256(newKey.PubKey().Bytes())
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		op := new(protocol.UpdateKeyOperation)

		op.OldEntry.KeyHash = kh[:]
		op.NewEntry.KeyHash = nkh[:]
		body := new(protocol.UpdateKeyPage)
		body.Operation = append(body.Operation, op)

		send(newTxn("foo/book1/1").
			WithSigner(url.MustParse("foo/book1/1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, testKey).
			Build())
	})

	page := n.GetKeyPage("foo/book1/1")
	require.Len(t, page.Keys, 1)
	require.Equal(t, nkh[:], page.Keys[0].PublicKeyHash)
}

func TestUpdateKey(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	fooKey, testKey, newKey := generateKey(), generateKey(), generateKey()
	newKeyHash := sha256.Sum256(newKey.PubKey().Bytes())
	testKeyHash := sha256.Sum256(testKey.PubKey().Bytes())
	_ = testKeyHash

	// UpdateKey should always be single-sig, so set the threshold to 2 and
	// ensure the transaction still succeeds.

	batch := n.db.Begin(true)
	defer batch.Discard()
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.AddCredits(batch, n.ParseUrl("foo/book1/1"), 1e9))
	require.NoError(t, acctesting.UpdateKeyPage(batch, url.MustParse("foo/book1/1"), func(p *protocol.KeyPage) { p.AcceptThreshold = 2 }))
	require.NoError(t, batch.Commit())

	spec := n.GetKeyPage("foo/book1/1")
	require.Len(t, spec.Keys, 1)
	require.Equal(t, testKeyHash[:], spec.Keys[0].PublicKeyHash)

	txnHashes := n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.UpdateKey)
		body.NewKeyHash = newKeyHash[:]

		send(newTxn("foo/book1/1").
			WithBody(body).
			WithSigner(url.MustParse("foo/book1/1"), 1).
			Initiate(protocol.SignatureTypeLegacyED25519, testKey).
			Build())
	})
	batch = n.db.Begin(false)
	defer batch.Discard()
	status, err := batch.Transaction(txnHashes[0][:]).GetStatus()
	require.NoError(t, err)
	require.False(t, status.Pending, "Transaction is still pending")

	spec = n.GetKeyPage("foo/book1/1")
	require.Len(t, spec.Keys, 1)
	require.Equal(t, newKeyHash[:], spec.Keys[0].PublicKeyHash)
}

func TestRemoveKey(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	fooKey, testKey1, testKey2 := generateKey(), generateKey(), generateKey()

	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", testKey1.PubKey().Bytes()))
	require.NoError(t, acctesting.AddCredits(batch, n.ParseUrl("foo/book1/1"), 1e9))
	require.NoError(t, batch.Commit())
	h2 := sha256.Sum256(testKey2.PubKey().Bytes())
	// Add second key because CreateKeyBook can't do it
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		op := new(protocol.AddKeyOperation)

		op.Entry.KeyHash = h2[:]
		body := new(protocol.UpdateKeyPage)
		body.Operation = append(body.Operation, op)

		send(newTxn("foo/book1/1").
			WithSigner(url.MustParse("foo/book1/1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, testKey1).
			Build())
	})
	h1 := sha256.Sum256(testKey1.PubKey().Bytes())
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		op := new(protocol.RemoveKeyOperation)

		op.Entry.KeyHash = h1[:]
		body := new(protocol.UpdateKeyPage)
		body.Operation = append(body.Operation, op)

		send(newTxn("foo/book1/1").
			WithSigner(url.MustParse("foo/book1/1"), 2).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, testKey2).
			Build())
	})

	page := n.GetKeyPage("foo/book1/1")
	require.Len(t, page.Keys, 1)
	require.Equal(t, h2[:], page.Keys[0].PublicKeyHash)
}

func TestSignatorHeight(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	liteKey, fooKey := generateKey(), generateKey()

	liteUrl, err := protocol.LiteTokenAddress(liteKey.PubKey().Bytes(), protocol.ACME, protocol.SignatureTypeED25519)
	require.NoError(t, err)
	tokenUrl, err := url.Parse("foo/tokens")
	require.NoError(t, err)
	keyBookUrl, err := url.Parse("foo/book")
	require.NoError(t, err)
	keyPageUrl := protocol.FormatKeyPageUrl(keyBookUrl, 0)
	require.NoError(t, err)

	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteKey, 1, 1e9))
	require.NoError(t, batch.Commit())

	getHeight := func(u *url.URL) uint64 {
		batch := n.db.Begin(true)
		defer batch.Discard()
		chain, err := batch.Account(u).ReadChain(protocol.MainChain)
		require.NoError(t, err)
		return uint64(chain.Height())
	}

	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = n.ParseUrl("foo")
		h := sha256.Sum256(fooKey.PubKey().Bytes())
		adi.KeyHash = h[:]
		adi.KeyBookUrl = keyBookUrl

		send(newTxn(liteUrl.RootIdentity().String()).
			WithBody(adi).
			Initiate(protocol.SignatureTypeLegacyED25519, liteKey).
			Build())
	})

	batch = n.db.Begin(true)
	require.NoError(t, acctesting.AddCredits(batch, keyPageUrl, 1e9))
	require.NoError(t, batch.Commit())

	keyPageHeight := getHeight(keyPageUrl)

	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		tac := new(protocol.CreateTokenAccount)
		tac.Url = tokenUrl
		tac.TokenUrl = protocol.AcmeUrl()
		send(newTxn("foo").
			WithSigner(protocol.FormatKeyPageUrl(keyBookUrl, 0), 1).
			WithBody(tac).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	require.Equal(t, keyPageHeight, getHeight(keyPageUrl), "Key page height changed")
}

func TestCreateToken(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	fooKey := generateKey()
	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, batch.Commit())

	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.CreateToken)
		body.Url = n.ParseUrl("foo/tokens")
		body.Symbol = "FOO"
		body.Precision = 10

		send(newTxn("foo").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	n.GetTokenIssuer("foo/tokens")
}

func TestIssueTokens(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	fooKey, liteKey := generateKey(), generateKey()
	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, acctesting.CreateTokenIssuer(batch, "foo/tokens", "FOO", 10, nil))
	require.NoError(t, batch.Commit())

	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], "foo/tokens", protocol.SignatureTypeED25519)
	require.NoError(t, err)

	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.IssueTokens)
		body.Recipient = liteAddr
		body.Amount.SetUint64(123)

		send(newTxn("foo/tokens").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	account := n.GetLiteTokenAccount(liteAddr.String())
	require.Equal(t, "acc://foo/tokens", account.TokenUrl.String())
	require.Equal(t, int64(123), account.Balance.Int64())
}

type CheckError struct {
	Disable bool
	H       func(err error)
}

func (c *CheckError) ErrorHandler() func(err error) {
	return func(err error) {
		if !c.Disable {
			c.H(err)
		}
	}
}

func TestIssueTokensWithSupplyLimit(t *testing.T) {
	check := CheckError{H: NewDefaultErrorHandler(t)}

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, check.ErrorHandler())
	n := nodes[subnets[1]][0]

	fooKey, liteKey := generateKey(), generateKey()
	sponsorUrl := acctesting.AcmeLiteAddressTmPriv(liteKey).RootIdentity()
	batch := n.db.Begin(true)

	fooDecimals := 10
	fooPrecision := uint64(math.Pow(10.0, float64(fooDecimals)))

	maxSupply := int64(1000000 * fooPrecision)
	supplyLimit := big.NewInt(maxSupply)
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, acctesting.CreateLiteIdentity(batch, sponsorUrl.String(), 3))
	require.NoError(t, acctesting.CreateLiteTokenAccount(batch, tmed25519.PrivKey(liteKey), 1e9))
	require.NoError(t, batch.Commit())

	var err error

	// issue tokens with supply limit
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.CreateToken)
		body.Url = n.ParseUrl("foo/tokens")
		body.Symbol = "FOO"
		body.Precision = uint64(fooDecimals)
		body.SupplyLimit = supplyLimit

		send(newTxn("foo").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	//test to make sure supply limit is set
	issuer := n.GetTokenIssuer("foo/tokens")
	require.Equal(t, supplyLimit.Int64(), issuer.SupplyLimit.Int64())

	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], "foo/tokens", protocol.SignatureTypeED25519)
	require.NoError(t, err)
	liteAcmeAddr, err := protocol.LiteTokenAddress(liteKey[32:], protocol.ACME, protocol.SignatureTypeED25519)
	require.NoError(t, err)
	liteId := liteAcmeAddr.RootIdentity()

	underLimit := int64(1000 * fooPrecision)
	atLimit := int64(maxSupply - underLimit)
	overLimit := int64(maxSupply + 1)
	// test under the limit
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.IssueTokens)
		body.Recipient = liteAddr

		body.Amount.SetInt64(underLimit)

		send(newTxn("foo/tokens").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	account := n.GetLiteTokenAccount(liteAddr.String())
	require.Equal(t, underLimit, account.Balance.Int64())

	issuer = n.GetTokenIssuer("foo/tokens")
	require.Equal(t, underLimit, issuer.Issued.Int64())
	//supply limit should not change
	require.Equal(t, maxSupply, issuer.SupplyLimit.Int64())

	// test at the limit
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.IssueTokens)
		body.Recipient = liteAddr

		body.Amount.SetInt64(atLimit)

		send(newTxn("foo/tokens").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	account = n.GetLiteTokenAccount(liteAddr.String())
	//the balance should now be at max supply
	require.Equal(t, maxSupply, account.Balance.Int64())

	//there should be no more tokens available in the tank
	issuer = n.GetTokenIssuer("foo/tokens")
	require.Equal(t, int64(0), issuer.SupplyLimit.Int64()-issuer.Issued.Int64())

	// test over the limit, this should fail, so tell fake tendermint not to give up
	// an error will be displayed on the console, but this is exactly what we expect so don't panic
	check.Disable = true
	_, _, err = n.Execute(func(send func(*protocol.Envelope)) {
		body := new(protocol.IssueTokens)
		body.Recipient = liteAddr

		body.Amount.SetInt64(overLimit)

		send(newTxn("foo/tokens").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	require.Error(t, err, "expected a failure but instead spending over the supply limit passed")

	account = n.GetLiteTokenAccount(liteAddr.String())
	//the balance should be equal to
	require.Greater(t, overLimit, account.Balance.Int64())

	//now lets buy some credits, so we can do a token burn
	check.Disable = false
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.AddCredits)
		//burn the underLimit amount to see if that gets returned to the pool
		body.Recipient = liteAddr.RootIdentity()
		body.Amount.SetUint64(100 * protocol.AcmePrecision)
		body.Oracle = n.GetOraclePrice()

		send(newTxn(liteAcmeAddr.String()).
			WithSigner(liteId.RootIdentity(), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, liteKey).
			Build())
	})

	//now lets burn some tokens to see if they get returned to the supply
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.BurnTokens)
		//burn the underLimit amount to see if that gets returned to the pool
		body.Amount.SetInt64(underLimit)

		send(newTxn(liteAddr.String()).
			WithSigner(liteAddr.RootIdentity(), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, liteKey).
			Build())
	})

	account = n.GetLiteTokenAccount(liteAddr.String())

	// previous balance was maxSupply, so test to make sure underLimit was debited
	require.Equal(t, maxSupply-underLimit, account.Balance.Int64())

	//there should now be the maxSupply - underLimit amount as the amount issued now
	issuer = n.GetTokenIssuer("foo/tokens")
	require.Equal(t, maxSupply-underLimit, issuer.Issued.Int64())

}

func TestInvalidDeposit(t *testing.T) {
	// The lite address ends with `foo/tokens` but the token is `foo2/tokens` so
	// the synthetic transaction will fail. This test verifies that the
	// transaction fails, but more importantly it verifies that
	// `Executor.Commit()` does *not* break if DeliverTx fails with a
	// non-existent origin. This is motivated by a bug that has been fixed. This
	// bug could have been triggered by a failing SyntheticCreateChains,
	// SyntheticDepositTokens, or SyntheticDepositCredits.

	t.Skip("TODO Fix - generate a receipt")

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	liteKey := generateKey()
	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], "foo/tokens", protocol.SignatureTypeED25519)
	require.NoError(t, err)

	id := n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.SyntheticDepositTokens)
		body.Source = n.network.NodeUrl()
		body.Token = n.ParseUrl("foo2/tokens")
		body.Amount.SetUint64(123)

		send(newTxn(liteAddr.String()).
			WithBody(body).
			InitiateSynthetic(n.network.NodeUrl()).
			Sign(protocol.SignatureTypeLegacyED25519, n.key.Bytes()).
			Build())
	})[0]

	tx := n.GetTx(id[:])
	require.NotZero(t, tx.Status.Code)
}

func DumpAccount(t *testing.T, batch *database.Batch, accountUrl *url.URL) {
	account := batch.Account(accountUrl)
	state, err := account.GetState()
	require.NoError(t, err)
	fmt.Println("Dump", accountUrl, state.Type())
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
			txState, err := batch.Transaction(id).GetState()
			require.NoError(t, err)
			txStatus, err := batch.Transaction(id).GetStatus()
			require.NoError(t, err)
			if seen[*(*[32]byte)(txState.Transaction.GetHash())] {
				fmt.Printf("      TX: hash=%X\n", txState.Transaction.GetHash())
				continue
			}
			fmt.Printf("      TX: type=%v origin=%v status=%#v\n", txState.Transaction.Body.Type(), txState.Transaction.Header.Principal, txStatus)
			seen[id32] = true
		}
	}
}

func TestUpdateValidators(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	netUrl := n.network.NodeUrl()
	validators := protocol.FormatKeyPageUrl(n.network.ValidatorBook(), 0)
	nodeKeyAdd1, nodeKeyAdd2, nodeKeyAdd3, nodeKeyUpd := generateKey(), generateKey(), generateKey(), generateKey()

	// Update NetworkGlobals - use 5/12 so that M = 1 for 3 validators and M = 2
	// for 4
	ng := new(protocol.NetworkGlobals)
	ng.ValidatorThreshold.Set(5, 12)
	wd := new(protocol.WriteData)
	d, err := ng.MarshalBinary()
	require.NoError(t, err)
	wd.Entry.Data = append(wd.Entry.Data, d)
	n.MustExecuteAndWait(func(send func(*Tx)) {
		send(newTxn(netUrl.JoinPath(protocol.Globals).String()).
			WithSigner(validators, 1).
			WithBody(wd).
			Initiate(protocol.SignatureTypeLegacyED25519, n.key.Bytes()).
			Build())
	})

	// Verify there is one validator (node key)
	require.ElementsMatch(t, n.client.Validators(), []crypto.PubKey{n.key.PubKey()})
	// Add a validator
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.AddValidator)
		body.PubKey = nodeKeyAdd1.PubKey().Bytes()
		send(newTxn(netUrl.JoinPath(protocol.ValidatorBook).String()).
			WithSigner(validators, 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, n.key.Bytes()).
			Build())
	})

	// Verify the validator was added
	require.ElementsMatch(t, n.client.Validators(), []crypto.PubKey{n.key.PubKey(), nodeKeyAdd1.PubKey()})

	// Update a validator
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.UpdateValidatorKey)

		body.PubKey = nodeKeyAdd1.PubKey().Bytes()
		body.NewPubKey = nodeKeyUpd.PubKey().Bytes()

		send(newTxn(netUrl.JoinPath(protocol.ValidatorBook).String()).
			WithSigner(validators, 2).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, n.key.Bytes()).
			Build())
	})

	// Verify the validator was updated
	require.ElementsMatch(t, n.client.Validators(), []crypto.PubKey{n.key.PubKey(), nodeKeyUpd.PubKey()})

	// Add a third validator
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.AddValidator)
		body.PubKey = nodeKeyAdd2.PubKey().Bytes()
		send(newTxn(netUrl.JoinPath(protocol.ValidatorBook).String()).
			WithSigner(validators, 3).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, n.key.Bytes()).
			Build())
	})

	// Verify the validator was added
	require.ElementsMatch(t, n.client.Validators(), []crypto.PubKey{n.key.PubKey(), nodeKeyUpd.PubKey(), nodeKeyAdd2.PubKey()})

	// Verify the Validator threshold
	require.Equal(t, uint64(1), n.GetKeyPage(validators.String()).AcceptThreshold)

	// Add a fourth validator, so the page threshold will become 2
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.AddValidator)
		body.PubKey = nodeKeyAdd3.PubKey().Bytes()

		send(newTxn(netUrl.JoinPath(protocol.ValidatorBook).String()).
			WithSigner(validators, 4).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, n.key.Bytes()).
			Build())
	})

	// Verify the validator was added
	require.ElementsMatch(t, n.client.Validators(), []crypto.PubKey{n.key.PubKey(), nodeKeyUpd.PubKey(), nodeKeyAdd2.PubKey(), nodeKeyAdd3.PubKey()})

	// Verify the Validator threshold
	require.Equal(t, uint64(2), n.GetKeyPage(validators.String()).AcceptThreshold)

	// Remove a validator
	txns := n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		body := new(protocol.RemoveValidator)
		body.PubKey = nodeKeyUpd.PubKey().Bytes()

		send(newTxn(netUrl.JoinPath(protocol.ValidatorBook).String()).
			WithSigner(validators, 5).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, n.key.Bytes()).
			Build())
	})

	envHashes, _ := n.MustExecute(func(send func(*protocol.Envelope)) {
		send(acctesting.NewTransaction().
			WithSigner(validators, 5).
			WithTxnHash(txns[0][:]).
			Sign(protocol.SignatureTypeED25519, nodeKeyAdd2.Bytes()).
			Build())
	})
	n.MustWaitForTxns(convertIds32(envHashes...)...)

	// Verify the validator was removed
	pubKeys := n.client.Validators()
	require.ElementsMatch(t, pubKeys, []crypto.PubKey{n.key.PubKey(), nodeKeyAdd2.PubKey(), nodeKeyAdd3.PubKey()})

}

func TestMultisig(t *testing.T) {
	check := CheckError{H: NewDefaultErrorHandler(t)}
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, check.ErrorHandler())

	key1, key2 := acctesting.GenerateTmKey(t.Name(), 1), acctesting.GenerateTmKey(t.Name(), 2)

	t.Log("Setup")
	n := nodes[subnets[1]][0]
	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, key1, "foo"))
	require.NoError(t, acctesting.UpdateKeyPage(batch, url.MustParse("foo/book0/1"), func(page *protocol.KeyPage) {
		hash := sha256.Sum256(key2[32:])
		page.AcceptThreshold = 2
		page.CreditBalance = 1e8
		page.Keys = append(page.Keys, &protocol.KeySpec{
			PublicKeyHash: hash[:],
		})
	}))
	require.NoError(t, batch.Commit())

	t.Log("Initiate the transaction")
	ids := n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		send(newTxn("foo").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(&protocol.CreateTokenAccount{
				Url:      url.MustParse("foo/tokens"),
				TokenUrl: protocol.AcmeUrl(),
			}).
			Initiate(protocol.SignatureTypeED25519, key1.Bytes()).
			Build())
	})

	txnResp := n.QueryTransaction(fmt.Sprintf("foo?txid=%X", ids[0]))
	require.False(t, txnResp.Status.Delivered, "Transaction is was delivered")
	require.True(t, txnResp.Status.Pending, "Transaction is not pending")

	t.Log("Double signing with key 1 should not complete the transaction")
	sigHashes, _ := n.MustExecute(func(send func(*protocol.Envelope)) {
		send(acctesting.NewTransaction().
			WithTimestampVar(&globalNonce).
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithTxnHash(ids[0][:]).
			Sign(protocol.SignatureTypeED25519, key1.Bytes()).
			Build())
	})
	n.MustWaitForTxns(convertIds32(sigHashes...)...)

	txnResp = n.QueryTransaction(fmt.Sprintf("foo?txid=%X", ids[0]))
	require.False(t, txnResp.Status.Delivered, "Transaction is was delivered")
	require.True(t, txnResp.Status.Pending, "Transaction is not pending")

	t.Log("Signing with key 2 should complete the transaction")
	sigHashes, _ = n.MustExecute(func(send func(*protocol.Envelope)) {
		send(acctesting.NewTransaction().
			WithTimestampVar(&globalNonce).
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithTxnHash(ids[0][:]).
			Sign(protocol.SignatureTypeED25519, key2.Bytes()).
			Build())
	})
	n.MustWaitForTxns(convertIds32(sigHashes...)...)

	txnResp = n.QueryTransaction(fmt.Sprintf("foo?txid=%X", ids[0]))
	require.True(t, txnResp.Status.Delivered, "Transaction is was not delivered")
	require.False(t, txnResp.Status.Pending, "Transaction is still pending")

	// this should fail, so tell fake tendermint not to give up
	// an error will be displayed on the console, but this is exactly what we expect so don't panic
	check.Disable = true
	t.Log("Signing a complete transaction should fail")
	_, _, err := n.Execute(func(send func(*protocol.Envelope)) {
		send(acctesting.NewTransaction().
			WithTimestampVar(&globalNonce).
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithTxnHash(ids[0][:]).
			Sign(protocol.SignatureTypeED25519, key2.Bytes()).
			Build())
	})
	require.Error(t, err)
}

func TestAccountAuth(t *testing.T) {
	check := CheckError{H: NewDefaultErrorHandler(t)}
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true, check.ErrorHandler())
	n := nodes[subnets[1]][0]

	fooKey, barKey := generateKey(), generateKey()
	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, acctesting.CreateTokenAccount(batch, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
	require.NoError(t, acctesting.CreateSubADI(batch, "foo", "foo/bar"))
	require.NoError(t, acctesting.CreateTokenAccount(batch, "foo/bar/tokens", protocol.AcmeUrl().String(), 0, false))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/bar/book", barKey.PubKey().Bytes()))
	require.NoError(t, acctesting.AddCredits(batch, url.MustParse("foo/bar/book/1"), 1e9))
	require.NoError(t, batch.Commit())

	// Disable auth
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		send(newTxn("foo/tokens").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(&protocol.UpdateAccountAuth{
				Operations: []protocol.AccountAuthOperation{
					&protocol.DisableAccountAuthOperation{
						Authority: url.MustParse("foo/book0"),
					},
				},
			}).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	// An unauthorized signer must not be allowed to enable auth
	check.Disable = true
	_, _, err := n.Execute(func(send func(*protocol.Envelope)) {
		send(newTxn("foo/tokens").
			WithSigner(url.MustParse("foo/bar/book/1"), 1).
			WithBody(&protocol.UpdateAccountAuth{
				Operations: []protocol.AccountAuthOperation{
					&protocol.EnableAccountAuthOperation{
						Authority: url.MustParse("foo/book0"),
					},
				},
			}).
			Initiate(protocol.SignatureTypeLegacyED25519, barKey).
			Build())
	})
	require.Error(t, err, "An unauthorized signer should not be able to enable auth")

	// An unauthorized signer should be able to send tokens
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		exch := new(protocol.SendTokens)
		exch.AddRecipient(n.ParseUrl("foo/bar/tokens"), big.NewInt(int64(68)))

		send(newTxn("foo/tokens").
			WithSigner(url.MustParse("foo/bar/book/1"), 1).
			WithBody(exch).
			Initiate(protocol.SignatureTypeLegacyED25519, barKey).
			Build())
	})

	require.Equal(t, int64(protocol.AcmePrecision-68), n.GetTokenAccount("foo/tokens").Balance.Int64())
	require.Equal(t, int64(68), n.GetTokenAccount("foo/bar/tokens").Balance.Int64())

	// Enable auth
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		send(newTxn("foo/tokens").
			WithSigner(url.MustParse("foo/book0/1"), 1).
			WithBody(&protocol.UpdateAccountAuth{
				Operations: []protocol.AccountAuthOperation{
					&protocol.EnableAccountAuthOperation{
						Authority: url.MustParse("foo/book0"),
					},
				},
			}).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	// An unauthorized signer should no longer be able to send tokens
	check.Disable = true
	_, _, err = n.Execute(func(send func(*protocol.Envelope)) {
		exch := new(protocol.SendTokens)
		exch.AddRecipient(n.ParseUrl("foo/bar/tokens"), big.NewInt(int64(68)))

		send(newTxn("foo/tokens").
			WithSigner(url.MustParse("foo/bar/book/1"), 1).
			WithBody(exch).
			Initiate(protocol.SignatureTypeLegacyED25519, barKey).
			Build())
	})
	require.Error(t, err, "expected a failure but instead an unauthorized signature succeeded")
}
