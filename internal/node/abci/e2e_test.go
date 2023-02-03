// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
	randpkg "golang.org/x/exp/rand"
)

var rand = randpkg.New(randpkg.NewSource(0))

type Tx = messaging.Envelope

func init() { acctesting.EnableDebugFeatures() }

func TestCreateLiteAccount(t *testing.T) {
	n := simulator.NewFakeNode(t, nil)

	const N, M = 11, 1
	const count = N * M
	credits := 10000.0
	originAddr, balances := testLiteTx(n, N, M, credits)
	amountSent := float64(count * 1000)
	initialAmount := protocol.AcmeFaucetAmount * protocol.AcmePrecision
	currentBalance := n.GetLiteTokenAccount(originAddr).Balance.Int64()
	totalAmountSent := initialAmount - amountSent
	require.Equal(t, int64(totalAmountSent), currentBalance)
	for addr, bal := range balances {
		require.Equal(t, bal, n.GetLiteTokenAccount(addr.String()).Balance.Int64())
	}
}

func testLiteTx(n *simulator.FakeNode, N, M int, credits float64) (string, map[*url.URL]int64) {
	sender := generateKey()
	senderUrl := acctesting.AcmeLiteAddressTmPriv(sender)
	recipients := make([]*url.URL, N)
	for i := range recipients {
		_, key, _ := ed25519.GenerateKey(nil)
		recipients[i] = acctesting.AcmeLiteAddressStdPriv(key)
	}

	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.AcmeFaucet)
		body.Url = senderUrl

		send(acctesting.NewTransaction().
			WithPrincipal(protocol.FaucetUrl.RootIdentity()).
			WithBody(body).
			Faucet())
	})

	n.Update(func(batch *database.Batch) {
		//acme to credits @ $0.05 acme price is 1:5
		liteTokenId := senderUrl.RootIdentity()
		n.Require().NoError(acctesting.AddCredits(batch, liteTokenId, credits))
	})

	balance := map[*url.URL]int64{}
	for i := 0; i < M; i++ {
		n.MustExecuteAndWait(func(send func(*Tx)) {
			for i := 0; i < N; i++ {
				recipient := recipients[rand.Intn(len(recipients))]
				balance[recipient] += 1000

				exch := new(protocol.SendTokens)
				exch.AddRecipient(recipient, big.NewInt(int64(1000)))
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
	n := simulator.NewFakeNode(t, nil)

	alice := generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
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
	n := simulator.NewFakeNode(t, nil)

	liteAccount := generateKey()
	newAdi := generateKey()
	keyHash := sha256.Sum256(newAdi.PubKey().Address())
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteAccount, protocol.AcmeFaucetAmount, 1e6))
	})

	n.MustExecuteAndWait(func(send func(*Tx)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = protocol.AccountUrl("RoadRunner")
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
	require.Equal(t, "acc://RoadRunner.acme", n.GetADI("RoadRunner").Url.String())

	// // Get the anchor chain manager
	// batch = n.db.Begin(true)
	// defer batch.Discard()
	// ledger := batch.Account(n.network.NodeUrl(protocol.Ledger))

	// // Check each anchor
	// // TODO FIX This is broken because the ledger no longer has a list of updates
	// var ledgerState *protocol.InternalLedger
	// require.NoError(t, ledger.GetStateAs(&ledgerState))
	// rootChain, err := ledger.MinorRootChain().Get()
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

	// // TODO Once block indexing has been implemented, verify that the following chains got modified
	// assert.Subset(t, accounts, []string{
	// 	"acc://RoadRunner#chain/main",
	// 	"acc://RoadRunner#chain/pending",
	// 	"acc://RoadRunner/book#chain/main",
	// 	"acc://RoadRunner/page#chain/main",
	// })
}

func TestCreateADI(t *testing.T) {
	n := simulator.NewFakeNode(t, nil)

	liteAccount := generateKey()
	newAdi := generateKey()
	keyHash := sha256.Sum256(newAdi.PubKey().Address())
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteAccount, protocol.AcmeFaucetAmount, 1e6))
	})

	n.MustExecuteAndWait(func(send func(*Tx)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = protocol.AccountUrl("RoadRunner")
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
	require.Equal(t, "acc://RoadRunner.acme", r.Url.String())

	kg := n.GetKeyBook("RoadRunner/foo-book")
	require.Equal(t, uint64(1), kg.PageCount)

	ks := n.GetKeyPage("RoadRunner/foo-book/1")
	require.Len(t, ks.Keys, 1)
	require.Equal(t, keyHash[:], ks.Keys[0].PublicKeyHash)
}

func TestAdiUrlLengthLimit(t *testing.T) {
	check := newDefaultCheckError(t, false)
	n := simulator.NewFakeNode(t, check.ErrorHandler())

	liteAccount := generateKey()
	newAdi := generateKey()
	keyHash := sha256.Sum256(newAdi.PubKey().Address())
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteAccount, protocol.AcmeFaucetAmount, 1e6))
	})
	accurl := strings.Repeat("𒀹", 250) // 𒀹 is 4 bytes

	txn := n.MustExecuteAndWait(func(send func(*Tx)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = protocol.AccountUrl(accurl)
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

	res := n.QueryTx(txn[0][:], time.Second, true)
	h := res.Produced.Records[0].Value.Hash()
	res = n.QueryTx(h[:], time.Second, true)
	require.Equal(t, errors.BadUrlLength, res.Status.Code)
}

func TestCreateADIWithoutKeybook(t *testing.T) {
	check := newDefaultCheckError(t, false)
	n := simulator.NewFakeNode(t, check.ErrorHandler())

	liteAccount := generateKey()
	newAdi := generateKey()
	keyHash := sha256.Sum256(newAdi.PubKey().Address())
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteAccount, protocol.AcmeFaucetAmount, 1e6))
	})

	_, _, err := n.Execute(func(send func(*Tx)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = protocol.AccountUrl("RoadRunner")
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

	firstEntry := protocol.AccumulateDataEntry{}

	firstEntry.Data = append(firstEntry.Data, []byte{})
	firstEntry.Data = append(firstEntry.Data, []byte("Factom PRO"))
	firstEntry.Data = append(firstEntry.Data, []byte("Tutorial"))

	//create a lite data account aka factom chainId
	chainId := protocol.ComputeLiteDataAccountId(&firstEntry)

	liteDataAddress, err := protocol.LiteDataAddress(chainId)
	if err != nil {
		t.Fatal(err)
	}

	n := simulator.NewFakeNode(t, nil)

	adiKey := generateKey()
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
	})
	ids := n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		wdt := new(protocol.WriteDataTo)
		wdt.Recipient = liteDataAddress
		wdt.Entry = &firstEntry
		send(newTxn("FooBar").
			WithSigner(protocol.AccountUrl("FooBar", "book0", "1"), 1).
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
	require.Equal(t, partialChainId, chainId)

	firstEntryHash := firstEntry.Hash()

	n.View(func(batch *database.Batch) {
		synthIds, err := batch.Transaction(ids[0][:]).GetSyntheticTxns()
		require.NoError(t, err)

		// Verify the entry hash in the transaction result
		h := synthIds.Entries[0].Hash()
		txStatus, err := batch.Transaction(h[:]).GetStatus()
		require.NoError(t, err)
		require.IsType(t, (*protocol.WriteDataResult)(nil), txStatus.Result)
		txResult := txStatus.Result.(*protocol.WriteDataResult)
		require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(txResult.EntryHash[:]), "Transaction result entry hash does not match")

		// Verify the entry hash returned by Entry
		entryHash, err := indexing.Data(batch, liteDataAddress).Entry(0)
		require.NoError(t, err)
		txnHash, err := indexing.Data(batch, liteDataAddress).Transaction(entryHash)
		require.NoError(t, err)
		entry, txId, causeTxId, err := indexing.GetDataEntry(batch, txnHash)
		require.NoError(t, err)
		hashFromEntry := entry.Hash()
		require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(hashFromEntry), "Chain Entry.Hash does not match")
		//sample verification for calculating the hash from lite data entry
		require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(entryHash), "Chain GetHashes does not match")
		require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(entry.Hash()), "Chain GetHashes does not match")
		require.NotNilf(t, txId, "txId not returned")
		require.NotNilf(t, causeTxId, "cause TxId not returned")
	})
}

func TestCreateAdiDataAccount(t *testing.T) {
	t.Run("Data Account with Default Key Book and no Manager Key Book", func(t *testing.T) {
		n := simulator.NewFakeNode(t, nil)

		adiKey := generateKey()
		n.Update(func(batch *database.Batch) {
			require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		})

		n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
			tac := new(protocol.CreateDataAccount)
			tac.Url = protocol.AccountUrl("FooBar", "oof")
			send(newTxn("FooBar").
				WithSigner(protocol.AccountUrl("FooBar", "book0", "1"), 1).
				WithBody(tac).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				Build())
		})

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, "acc://FooBar.acme/oof", r.Url.String())

		require.Contains(t, n.GetDirectory("FooBar"), protocol.AccountUrl("FooBar", "oof").String())
	})

	t.Run("Data Account with Custom Key Book and Manager Key Book Url", func(t *testing.T) {
		n := simulator.NewFakeNode(t, nil)

		adiKey, pageKey := generateKey(), generateKey()
		n.Update(func(batch *database.Batch) {
			require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
			require.NoError(t, acctesting.CreateKeyBook(batch, "acc://FooBar/foo/book1", pageKey.PubKey().Bytes()))
			require.NoError(t, acctesting.CreateKeyPage(batch, "acc://FooBar/foo/book1"))
			require.NoError(t, acctesting.CreateKeyBook(batch, "acc://FooBar/mgr/book1", nil))
			require.NoError(t, acctesting.CreateKeyPage(batch, "acc://FooBar/mgr/book1", pageKey.PubKey().Bytes()))
			require.NoError(t, acctesting.AddCredits(batch, protocol.AccountUrl("FooBar", "foo", "book1", "1"), 1e9))
			require.NoError(t, acctesting.AddCredits(batch, protocol.AccountUrl("FooBar", "mgr", "book1", "2"), 1e9))
		})

		n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
			cda := new(protocol.CreateDataAccount)
			cda.Url = protocol.AccountUrl("FooBar", "oof")
			cda.Authorities = []*url.URL{
				protocol.AccountUrl("FooBar", "foo", "book1"),
				protocol.AccountUrl("FooBar", "mgr", "book1"),
			}
			send(newTxn("FooBar").
				WithSigner(protocol.AccountUrl("FooBar", "book0", "1"), 1).
				WithBody(cda).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				WithSigner(protocol.AccountUrl("FooBar", "foo", "book1", "1"), 1).
				Sign(protocol.SignatureTypeED25519, pageKey).
				WithSigner(protocol.AccountUrl("FooBar", "mgr", "book1", "2"), 1).
				Sign(protocol.SignatureTypeED25519, pageKey).
				Build())
		})

		u := protocol.AccountUrl("FooBar", "foo", "book1")

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, "acc://FooBar.acme/oof", r.Url.String())
		require.Equal(t, "acc://FooBar.acme/mgr/book1", r.ManagerKeyBook().String())
		require.Equal(t, u.String(), r.KeyBook().String())

	})

	t.Run("Data Account data entry", func(t *testing.T) {
		n := simulator.NewFakeNode(t, nil)

		adiKey := generateKey()
		n.Update(func(batch *database.Batch) {
			require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		})

		n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
			tac := new(protocol.CreateDataAccount)
			tac.Url = protocol.AccountUrl("FooBar", "oof")
			send(newTxn("FooBar").
				WithSigner(protocol.AccountUrl("FooBar", "book0", "1"), 1).
				WithBody(tac).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				Build())
		})

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, "acc://FooBar.acme/oof", r.Url.String())
		require.Contains(t, n.GetDirectory("FooBar"), protocol.AccountUrl("FooBar", "oof").String())

		wd := new(protocol.WriteData)
		n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
			entry := new(protocol.AccumulateDataEntry)
			wd.Entry = entry
			entry.Data = append(entry.Data, []byte("thequickbrownfoxjumpsoverthelazydog"))
			for i := 0; i < 10; i++ {
				entry.Data = append(entry.Data, []byte(fmt.Sprintf("test id %d", i)))
			}

			send(newTxn("FooBar/oof").
				WithSigner(protocol.AccountUrl("FooBar", "book0", "1"), 1).
				WithBody(wd).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				Build())
		})

		// Test getting the data by URL
		rde := n.GetDataEntry("FooBar/oof", nil)
		if !protocol.EqualDataEntry(rde, wd.Entry) {
			t.Fatalf("data query does not match what was entered")
		}

		//now test query by entry hash.
		rde2 := n.GetDataEntry("FooBar/oof", &api.DataQuery{Entry: wd.Entry.Hash()})
		if !protocol.EqualDataEntry(rde2, wd.Entry) {
			t.Fatalf("data query does not match what was entered")
		}

		//now test query by entry set
		var zero uint64
		rde3 := n.GetDataEntry("FooBar/oof", &api.DataQuery{Index: &zero})
		if !protocol.EqualDataEntry(rde3, wd.Entry) {
			t.Fatalf("data query does not match what was entered")
		}
	})

	t.Run("Data Account data entry to scratch chain", func(t *testing.T) {
		n := simulator.NewFakeNode(t, nil)

		adiKey := generateKey()
		n.Update(func(batch *database.Batch) {
			require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		})

		n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
			tac := new(protocol.CreateDataAccount)
			tac.Url = protocol.AccountUrl("FooBar", "scr")
			send(newTxn("FooBar").
				WithSigner(protocol.AccountUrl("FooBar", "book0", "1"), 1).
				WithBody(tac).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				Build())
		})

		r := n.GetDataAccount("FooBar/scr")
		require.Equal(t, "acc://FooBar.acme/scr", r.Url.String())
		require.Contains(t, n.GetDirectory("FooBar"), protocol.AccountUrl("FooBar", "scr").String())

		wd := new(protocol.WriteData)
		wd.Scratch = true
		n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
			entry := new(protocol.AccumulateDataEntry)
			wd.Entry = entry
			entry.Data = append(entry.Data, []byte("thequickbrownfoxjumpsoverthelazydog"))
			for i := 0; i < 10; i++ {
				entry.Data = append(entry.Data, []byte(fmt.Sprintf("test id %d", i)))
			}

			send(newTxn("FooBar/scr").
				WithSigner(protocol.AccountUrl("FooBar", "book0", "1"), 1).
				WithBody(wd).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				Build())
		})

		// Test getting the data by URL
		rde := n.GetDataEntry("FooBar/scr", nil)
		if !protocol.EqualDataEntry(rde, wd.Entry) {
			t.Fatalf("data query does not match what was entered")
		}

		//now test query by entry hash.
		rde2 := n.GetDataEntry("FooBar/scr", &api.DataQuery{Entry: wd.Entry.Hash()})
		if !protocol.EqualDataEntry(rde2, wd.Entry) {
			t.Fatalf("data query does not match what was entered")
		}

		//now test query by entry set
		var zero uint64
		rde3 := n.GetDataEntry("FooBar/scr", &api.DataQuery{Index: &zero})
		if !protocol.EqualDataEntry(rde3, wd.Entry) {
			t.Fatalf("data query does not match what was entered")
		}
	})
}

func TestCreateAdiTokenAccount(t *testing.T) {
	t.Run("Default Key Book", func(t *testing.T) {
		n := simulator.NewFakeNode(t, nil)

		adiKey := generateKey()
		n.Update(func(batch *database.Batch) {
			require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
		})

		n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
			tac := new(protocol.CreateTokenAccount)
			tac.Url = protocol.AccountUrl("FooBar", "Baz")
			tac.TokenUrl = protocol.AcmeUrl()
			send(newTxn("FooBar").
				WithSigner(protocol.AccountUrl("FooBar", "book0", "1"), 1).
				WithBody(tac).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				Build())
		})

		r := n.GetTokenAccount("FooBar/Baz")
		require.Equal(t, "acc://FooBar.acme/Baz", r.Url.String())
		require.Equal(t, protocol.AcmeUrl().String(), r.TokenUrl.String())

		require.ElementsMatch(t, []string{
			protocol.AccountUrl("FooBar", "book0").String(),
			protocol.AccountUrl("FooBar", "book0", "1").String(),
			protocol.AccountUrl("FooBar", "Baz").String(),
		}, n.GetDirectory("FooBar"))
	})

	t.Run("Custom Key Book", func(t *testing.T) {
		n := simulator.NewFakeNode(t, nil)

		adiKey, pageKey := generateKey(), generateKey()
		n.Update(func(batch *database.Batch) {
			require.NoError(t, acctesting.CreateAdiWithCredits(batch, adiKey, "FooBar", 1e9))
			require.NoError(t, acctesting.CreateKeyBook(batch, "FooBar/book1", pageKey.PubKey().Bytes()))
			require.NoError(t, acctesting.AddCredits(batch, protocol.AccountUrl("FooBar", "book1", "1"), 1e9))
		})

		n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
			tac := new(protocol.CreateTokenAccount)
			tac.Url = protocol.AccountUrl("FooBar", "Baz")
			tac.TokenUrl = protocol.AcmeUrl()
			tac.Authorities = []*url.URL{protocol.AccountUrl("FooBar", "book1")}
			send(newTxn("FooBar").
				WithSigner(protocol.AccountUrl("FooBar", "book0", "1"), 1).
				WithBody(tac).
				Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
				WithSigner(protocol.AccountUrl("FooBar", "book1", "1"), 1).
				Sign(protocol.SignatureTypeED25519, pageKey).
				Build())
		})
	})

	t.Run("Remote Key Book", func(t *testing.T) {
		n := simulator.NewFakeNode(t, nil)

		aliceKey, bobKey := generateKey(), generateKey()
		n.Update(func(batch *database.Batch) {
			require.NoError(t, acctesting.CreateAdiWithCredits(batch, aliceKey, "alice", 1e9))
			require.NoError(t, acctesting.CreateAdiWithCredits(batch, bobKey, "bob", 1e9))
		})

		n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
			tac := new(protocol.CreateTokenAccount)
			tac.Url = protocol.AccountUrl("alice", "tokens")
			tac.TokenUrl = protocol.AcmeUrl()
			tac.Authorities = []*url.URL{protocol.AccountUrl("bob", "book0")}
			send(newTxn("alice").
				WithSigner(protocol.AccountUrl("alice", "book0", "1"), 1).
				WithBody(tac).
				Initiate(protocol.SignatureTypeLegacyED25519, aliceKey).
				WithSigner(protocol.AccountUrl("bob", "book0", "1"), 1).
				Sign(protocol.SignatureTypeED25519, bobKey).
				Build())
		})

		// Wait for the remote signature to settle
		time.Sleep(time.Second)

		r := n.GetTokenAccount("alice/tokens")
		require.Equal(t, "alice.acme/tokens", r.Url.ShortString())
		require.Equal(t, protocol.AcmeUrl().String(), r.TokenUrl.String())
		require.Equal(t, "bob.acme/book0", r.KeyBook().ShortString())
	})
}

func TestLiteAccountTx(t *testing.T) {
	n := simulator.NewFakeNode(t, nil)

	alice, bob, charlie := generateKey(), generateKey(), generateKey()
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, protocol.AcmeFaucetAmount, 1e9))
		require.NoError(t, acctesting.CreateLiteTokenAccount(batch, bob, 0))
		require.NoError(t, acctesting.CreateLiteTokenAccount(batch, charlie, 0))
	})

	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	bobUrl := acctesting.AcmeLiteAddressTmPriv(bob)
	charlieUrl := acctesting.AcmeLiteAddressTmPriv(charlie)

	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		exch := new(protocol.SendTokens)
		exch.AddRecipient(bobUrl, big.NewInt(int64(1000)))
		exch.AddRecipient(charlieUrl, big.NewInt(int64(2000)))

		send(newTxn(aliceUrl.String()).
			WithSigner(aliceUrl.RootIdentity(), 1).
			WithBody(exch).
			Initiate(protocol.SignatureTypeLegacyED25519, alice).
			Build())
	})

	require.Equal(t, int64(protocol.AcmeFaucetAmount*protocol.AcmePrecision-3000), n.GetLiteTokenAccount(aliceUrl.String()).Balance.Int64())
	require.Equal(t, int64(1000), n.GetLiteTokenAccount(bobUrl.String()).Balance.Int64())
	require.Equal(t, int64(2000), n.GetLiteTokenAccount(charlieUrl.String()).Balance.Int64())
}

func TestAdiAccountTx(t *testing.T) {
	n := simulator.NewFakeNode(t, nil)

	fooKey, barKey := generateKey(), generateKey()
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
		require.NoError(t, acctesting.CreateTokenAccount(batch, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
		require.NoError(t, acctesting.CreateADI(batch, barKey, "bar"))
		require.NoError(t, acctesting.CreateTokenAccount(batch, "bar/tokens", protocol.AcmeUrl().String(), 0, false))
	})

	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		exch := new(protocol.SendTokens)
		exch.AddRecipient(protocol.AccountUrl("bar", "tokens"), big.NewInt(int64(68)))

		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
			WithBody(exch).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	require.Equal(t, int64(protocol.AcmePrecision-68), n.GetTokenAccount("foo/tokens").Balance.Int64())
	require.Equal(t, int64(68), n.GetTokenAccount("bar/tokens").Balance.Int64())
}

func TestSendTokensToBadRecipient(t *testing.T) {
	check := newDefaultCheckError(t, false)
	n := simulator.NewFakeNode(t, check.ErrorHandler())

	alice := generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, protocol.AcmeFaucetAmount, 1e9))
	})

	// The send should succeed
	txnHashes := n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		exch := new(protocol.SendTokens)
		exch.AddRecipient(protocol.AccountUrl("foo"), big.NewInt(int64(1000)))

		send(newTxn(aliceUrl.String()).
			WithSigner(aliceUrl.RootIdentity(), 1).
			WithBody(exch).
			Initiate(protocol.SignatureTypeLegacyED25519, alice).
			Build())
	})

	// The synthetic transaction should fail
	res := n.QueryTx(txnHashes[0][:], time.Second, true)
	h := res.Produced.Records[0].Value.Hash()
	res = n.QueryTx(h[:], time.Second, true)
	require.Equal(t, errors.NotFound, res.Status.Code)

	// Give the synthetic receipt a second to resolve - workaround AC-1238
	time.Sleep(time.Second)

	// The balance should be unchanged
	require.Equal(t, int64(protocol.AcmeFaucetAmount*protocol.AcmePrecision), n.GetLiteTokenAccount(aliceUrl.String()).Balance.Int64())
}

func TestCreateKeyPage(t *testing.T) {
	n := simulator.NewFakeNode(t, nil)

	fooKey, testKey := generateKey(), generateKey()
	fkh := sha256.Sum256(fooKey.PubKey().Bytes())
	tkh := sha256.Sum256(testKey.PubKey().Bytes())
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	})

	page := n.GetKeyPage("foo/book0/1")
	require.Len(t, page.Keys, 1)
	key := page.Keys[0]
	require.Equal(t, uint64(0), key.LastUsedOn)
	require.Equal(t, fkh[:], key.PublicKeyHash)

	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		cms := new(protocol.CreateKeyPage)
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			KeyHash: tkh[:],
		})

		send(newTxn("foo/book0").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
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
	n := simulator.NewFakeNode(t, nil)

	fooKey, testKey := generateKey(), generateKey()
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	})

	bookUrl := protocol.AccountUrl("foo", "book1")
	pageUrl := protocol.AccountUrl("foo", "book1", "1")

	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		csg := new(protocol.CreateKeyBook)
		csg.Url = protocol.AccountUrl("foo", "book1")
		csg.PublicKeyHash = testKey.PubKey().Bytes()

		send(newTxn("foo").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
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
	n := simulator.NewFakeNode(t, nil)

	fooKey, testKey1, testKey2 := generateKey(), generateKey(), generateKey()

	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
		require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", testKey1.PubKey().Bytes()))
		require.NoError(t, acctesting.AddCredits(batch, protocol.AccountUrl("foo", "book1", "1"), 1e9))
	})

	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		cms := new(protocol.CreateKeyPage)
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			KeyHash: testKey2.PubKey().Bytes(),
		})

		send(newTxn("foo/book1").
			WithSigner(protocol.AccountUrl("foo", "book1", "1"), 1).
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
	n := simulator.NewFakeNode(t, nil)

	fooKey, testKey := generateKey(), generateKey()

	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
		require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", testKey.PubKey().Bytes()))
		require.NoError(t, acctesting.AddCredits(batch, protocol.AccountUrl("foo", "book1", "1"), 1e9))
	})

	newKey := generateKey()
	nkh := sha256.Sum256(newKey.PubKey().Bytes())

	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		op := new(protocol.AddKeyOperation)
		op.Entry.KeyHash = nkh[:]
		body := new(protocol.UpdateKeyPage)
		body.Operation = append(body.Operation, op)

		send(newTxn("foo/book1/1").
			WithSigner(protocol.AccountUrl("foo", "book1", "1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, testKey).
			Build())
	})

	page := n.GetKeyPage("foo/book1/1")
	require.Len(t, page.Keys, 2)
	//look for the key.
	_, _, found := page.EntryByKeyHash(nkh[:])
	require.True(t, found, "key not found in page")
}

func TestUpdateKeyPage(t *testing.T) {
	n := simulator.NewFakeNode(t, nil)

	fooKey, testKey := generateKey(), generateKey()

	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
		require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", testKey.PubKey().Bytes()))
		require.NoError(t, acctesting.AddCredits(batch, protocol.AccountUrl("foo", "book1", "1"), 1e9))
	})

	newKey := generateKey()
	kh := sha256.Sum256(testKey.PubKey().Bytes())
	nkh := sha256.Sum256(newKey.PubKey().Bytes())
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		op := new(protocol.UpdateKeyOperation)

		op.OldEntry.KeyHash = kh[:]
		op.NewEntry.KeyHash = nkh[:]
		body := new(protocol.UpdateKeyPage)
		body.Operation = append(body.Operation, op)

		send(newTxn("foo/book1/1").
			WithSigner(protocol.AccountUrl("foo", "book1", "1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, testKey).
			Build())
	})

	page := n.GetKeyPage("foo/book1/1")
	require.Len(t, page.Keys, 1)
	require.Equal(t, nkh[:], page.Keys[0].PublicKeyHash)
}

func TestUpdateKey(t *testing.T) {
	n := simulator.NewFakeNode(t, nil)

	fooKey, testKey, newKey := generateKey(), generateKey(), generateKey()
	newKeyHash := sha256.Sum256(newKey.PubKey().Bytes())
	testKeyHash := sha256.Sum256(testKey.PubKey().Bytes())
	_ = testKeyHash

	// UpdateKey should always be single-sig, so set the threshold to 2 and
	// ensure the transaction still succeeds.

	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
		require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", testKey.PubKey().Bytes()))
		require.NoError(t, acctesting.AddCredits(batch, protocol.AccountUrl("foo", "book1", "1"), 1e9))
		require.NoError(t, acctesting.UpdateKeyPage(batch, protocol.AccountUrl("foo", "book1", "1"), func(p *protocol.KeyPage) { p.AcceptThreshold = 2 }))
	})

	spec := n.GetKeyPage("foo/book1/1")
	require.Len(t, spec.Keys, 1)
	require.Equal(t, testKeyHash[:], spec.Keys[0].PublicKeyHash)

	txnHashes := n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.UpdateKey)
		body.NewKeyHash = newKeyHash[:]

		send(newTxn("foo/book1/1").
			WithBody(body).
			WithSigner(protocol.AccountUrl("foo", "book1", "1"), 1).
			Initiate(protocol.SignatureTypeLegacyED25519, testKey).
			Build())
	})
	r := n.QueryTx(txnHashes[0][:], 0, false)
	require.False(t, r.Status.Pending(), "Transaction is still pending")

	spec = n.GetKeyPage("foo/book1/1")
	require.Len(t, spec.Keys, 1)
	require.Equal(t, newKeyHash[:], spec.Keys[0].PublicKeyHash)
}

func TestRemoveKey(t *testing.T) {
	n := simulator.NewFakeNode(t, nil)

	fooKey, testKey1, testKey2 := generateKey(), generateKey(), generateKey()

	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
		require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book1", testKey1.PubKey().Bytes()))
		require.NoError(t, acctesting.AddCredits(batch, protocol.AccountUrl("foo", "book1", "1"), 1e9))
	})
	h2 := sha256.Sum256(testKey2.PubKey().Bytes())
	// Add second key because CreateKeyBook can't do it
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		op := new(protocol.AddKeyOperation)

		op.Entry.KeyHash = h2[:]
		body := new(protocol.UpdateKeyPage)
		body.Operation = append(body.Operation, op)

		send(newTxn("foo/book1/1").
			WithSigner(protocol.AccountUrl("foo", "book1", "1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, testKey1).
			Build())
	})
	h1 := sha256.Sum256(testKey1.PubKey().Bytes())
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		op := new(protocol.RemoveKeyOperation)

		op.Entry.KeyHash = h1[:]
		body := new(protocol.UpdateKeyPage)
		body.Operation = append(body.Operation, op)

		send(newTxn("foo/book1/1").
			WithSigner(protocol.AccountUrl("foo", "book1", "1"), 2).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, testKey2).
			Build())
	})

	page := n.GetKeyPage("foo/book1/1")
	require.Len(t, page.Keys, 1)

	//look for the H1 key, which should have been removed
	_, _, found := page.EntryByKeyHash(h1[:])
	require.False(t, found, "key was found in page when it should have been removed")

	//look for the H2 key which was also added before H1 was removed
	_, _, found = page.EntryByKeyHash(h2[:])
	require.True(t, found, "key was not found in page it was expected to be in")
}

func TestSignatorHeight(t *testing.T) {
	n := simulator.NewFakeNode(t, nil)

	liteKey, fooKey := generateKey(), generateKey()

	liteUrl, err := protocol.LiteTokenAddress(liteKey.PubKey().Bytes(), protocol.ACME, protocol.SignatureTypeED25519)
	require.NoError(t, err)
	tokenUrl := protocol.AccountUrl("foo", "tokens")
	keyBookUrl := protocol.AccountUrl("foo", "book")
	keyPageUrl := protocol.FormatKeyPageUrl(keyBookUrl, 0)
	require.NoError(t, err)
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteKey, 1, 1e9))
	})

	getHeight := func(u *url.URL) (h uint64) {
		n.View(func(batch *database.Batch) {
			chain, err := batch.Account(u).MainChain().Get()
			require.NoError(t, err)
			h = uint64(chain.Height())
		})
		return
	}

	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = protocol.AccountUrl("foo")
		h := sha256.Sum256(fooKey.PubKey().Bytes())
		adi.KeyHash = h[:]
		adi.KeyBookUrl = keyBookUrl

		send(newTxn(liteUrl.RootIdentity().String()).
			WithBody(adi).
			Initiate(protocol.SignatureTypeLegacyED25519, liteKey).
			Build())
	})

	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.AddCredits(batch, keyPageUrl, 1e9))
	})

	keyPageHeight := getHeight(keyPageUrl)

	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
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
	n := simulator.NewFakeNode(t, nil)

	fooKey := generateKey()
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	})

	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.CreateToken)
		body.Url = protocol.AccountUrl("foo", "tokens")
		body.Symbol = "FOO"
		body.Precision = 10

		send(newTxn("foo").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	n.GetTokenIssuer("foo/tokens")
}

func TestIssueTokens(t *testing.T) {
	check := newDefaultCheckError(t, true)
	n := simulator.NewFakeNode(t, check.ErrorHandler())

	fooKey, liteKey := generateKey(), generateKey()
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
		require.NoError(t, acctesting.CreateTokenIssuer(batch, "foo/tokens", "FOO", 10, nil))
		require.NoError(t, acctesting.CreateTokenAccount(batch, "foo.acme/acmetokens", "acc://ACME", float64(10), false))
		require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteKey, 1, 1e9))
	})
	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], "foo.acme/tokens", protocol.SignatureTypeED25519)
	require.NoError(t, err)

	// Issue foo.acme/tokens to a foo.acme/tokens lite token account
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.IssueTokens)
		body.Recipient = liteAddr
		body.Amount.SetUint64(123)

		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	// Verify tokens were received
	account := n.GetLiteTokenAccount(liteAddr.String())
	require.Equal(t, "acc://foo.acme/tokens", account.TokenUrl.String())
	require.Equal(t, int64(123), account.Balance.Int64())

	// Issue foo.acme/tokens to an ACME token account
	check.Disable = true
	initialbalance := n.GetTokenAccount("acc://foo.acme/acmetokens").Balance
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.IssueTokens)
		body.Recipient = n.GetTokenAccount("acc://foo.acme/acmetokens").Url
		body.Amount.SetUint64(123)

		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	// Verify tokens were not received
	finalbalance := n.GetTokenAccount("acc://foo.acme/acmetokens").Balance
	require.Equal(t, initialbalance, finalbalance)
}

func TestIssueTokensRefund(t *testing.T) {
	check := newDefaultCheckError(t, true)

	n := simulator.NewFakeNode(t, check.ErrorHandler())

	fooKey, liteKey := generateKey(), generateKey()
	sponsorUrl := acctesting.AcmeLiteAddressTmPriv(liteKey).RootIdentity()
	fooDecimals := 10
	fooPrecision := uint64(math.Pow(10.0, float64(fooDecimals)))

	maxSupply := int64(1000000 * fooPrecision)
	supplyLimit := big.NewInt(maxSupply)

	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
		require.NoError(t, acctesting.CreateLiteIdentity(batch, sponsorUrl.String(), 3))
		require.NoError(t, acctesting.CreateLiteTokenAccount(batch, liteKey, 1e9))
	})
	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], "foo.acme/tokens", protocol.SignatureTypeED25519)
	require.NoError(t, err)

	// issue tokens with supply limit
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.CreateToken)
		body.Url = protocol.AccountUrl("foo", "tokens")
		body.Symbol = "FOO"
		body.Precision = uint64(fooDecimals)
		body.SupplyLimit = supplyLimit

		send(newTxn("foo").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	//test to make sure supply limit is set
	issuer := n.GetTokenIssuer("foo/tokens")
	require.Equal(t, supplyLimit.Int64(), issuer.SupplyLimit.Int64())

	//issue tokens successfully
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.IssueTokens)
		body.Recipient = liteAddr
		body.Amount.SetUint64(123)

		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})
	issuer = n.GetTokenIssuer("foo/tokens")
	require.Equal(t, int64(123), issuer.Issued.Int64())

	account := n.GetLiteTokenAccount(liteAddr.String())
	require.Equal(t, "acc://foo.acme/tokens", account.TokenUrl.String())
	require.Equal(t, int64(123), account.Balance.Int64())

	//issue tokens to incorrect principal
	check.Disable = true
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.IssueTokens)
		liteAddr = liteAddr.WithAuthority(liteAddr.Authority + "u")
		body.Recipient = liteAddr
		body.Amount.SetUint64(123)

		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})
	issuer = n.GetTokenIssuer("foo/tokens")
	require.Equal(t, int64(123), issuer.Issued.Int64())
}

type CheckError struct {
	T       *testing.T
	Disable bool
	H       func(err error)
}

func newDefaultCheckError(t *testing.T, enable bool) *CheckError {
	return &CheckError{T: t, H: func(err error) {
		t.Helper()

		var err2 *errors.Error
		if errors.As(err, &err2) && err2.Code == errors.Delivered {
			return
		}
		assert.NoError(t, err)
	}, Disable: !enable}
}

func (c *CheckError) ErrorHandler() func(err error) {
	return func(err error) {
		c.T.Helper()
		if !c.Disable {
			c.H(err)
		}
	}
}

func TestIssueTokensWithSupplyLimit(t *testing.T) {
	check := newDefaultCheckError(t, true)

	n := simulator.NewFakeNode(t, check.ErrorHandler())

	fooKey, liteKey := generateKey(), generateKey()
	sponsorUrl := acctesting.AcmeLiteAddressTmPriv(liteKey).RootIdentity()
	fooDecimals := 10
	fooPrecision := uint64(math.Pow(10.0, float64(fooDecimals)))

	maxSupply := int64(1000000 * fooPrecision)
	supplyLimit := big.NewInt(maxSupply)

	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
		require.NoError(t, acctesting.CreateLiteIdentity(batch, sponsorUrl.String(), 3))
		require.NoError(t, acctesting.CreateLiteTokenAccount(batch, liteKey, 1e9))
	})

	var err error

	// issue tokens with supply limit
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.CreateToken)
		body.Url = protocol.AccountUrl("foo", "tokens")
		body.Symbol = "FOO"
		body.Precision = uint64(fooDecimals)
		body.SupplyLimit = supplyLimit

		send(newTxn("foo").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	//test to make sure supply limit is set
	issuer := n.GetTokenIssuer("foo/tokens")
	require.Equal(t, supplyLimit.Int64(), issuer.SupplyLimit.Int64())

	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], "foo.acme/tokens", protocol.SignatureTypeED25519)
	require.NoError(t, err)
	liteAcmeAddr, err := protocol.LiteTokenAddress(liteKey[32:], protocol.ACME, protocol.SignatureTypeED25519)
	require.NoError(t, err)
	liteId := liteAcmeAddr.RootIdentity()

	underLimit := int64(1000 * fooPrecision)
	atLimit := maxSupply - underLimit
	overLimit := maxSupply + 1
	// test under the limit
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.IssueTokens)
		body.Recipient = liteAddr

		body.Amount.SetInt64(underLimit)

		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
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
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.IssueTokens)
		body.Recipient = liteAddr

		body.Amount.SetInt64(atLimit)

		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
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
	_, _, err = n.Execute(func(send func(*messaging.Envelope)) {
		body := new(protocol.IssueTokens)
		body.Recipient = liteAddr

		body.Amount.SetInt64(overLimit)

		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
			WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	require.Error(t, err, "expected a failure but instead spending over the supply limit passed")
	time.Sleep(time.Second) // The test fails if this is removed

	account = n.GetLiteTokenAccount(liteAddr.String())
	//the balance should be equal to
	require.Greater(t, overLimit, account.Balance.Int64())

	//now lets buy some credits, so we can do a token burn
	check.Disable = false
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
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
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
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

func DumpAccount(t *testing.T, batch *database.Batch, accountUrl *url.URL) {
	account := batch.Account(accountUrl)
	state, err := account.GetState()
	require.NoError(t, err)
	fmt.Println("Dump", accountUrl, state.Type())
	chains, err := account.Chains().Get()
	require.NoError(t, err)
	seen := map[[32]byte]bool{}
	for _, cmeta := range chains {
		chain, err := account.GetChainByName(cmeta.Name)
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

// func TestMultisig(t *testing.T) {
// 	check := newDefaultCheckError(t, true)
// 	n := simulator.NewFakeNode(t, check.ErrorHandler())

// 	key1, key2 := acctesting.GenerateTmKey(t.Name(), 1), acctesting.GenerateTmKey(t.Name(), 2)

// 	t.Log("Setup")
// 	n.Update(func(batch *database.Batch) {
// 		require.NoError(t, acctesting.CreateADI(batch, key1, "foo"))
// 		require.NoError(t, acctesting.UpdateKeyPage(batch, protocol.AccountUrl("foo", "book0", "1"), func(page *protocol.KeyPage) {
// 			hash := sha256.Sum256(key2[32:])
// 			page.AcceptThreshold = 2
// 			page.CreditBalance = 1e8
// 			page.AddKeySpec(&protocol.KeySpec{
// 				PublicKeyHash: hash[:],
// 			})
// 		}))
// 	})

// 	t.Log("Initiate the transaction")
// 	ids := n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
// 		send(newTxn("foo").
// 			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
// 			WithBody(&protocol.CreateTokenAccount{
// 				Url:      protocol.AccountUrl("foo", "tokens"),
// 				TokenUrl: protocol.AcmeUrl(),
// 			}).
// 			Initiate(protocol.SignatureTypeED25519, key1.Bytes()).
// 			Build())
// 	})

// 	txnResp := n.queryTransaction(fmt.Sprintf("foo?txid=%X", ids[0]))
// 	require.False(t, txnResp.Status.Delivered(), "Transaction is was delivered")
// 	require.True(t, txnResp.Status.Pending(), "Transaction is not pending")

// 	t.Log("Double signing with key 1 should not complete the transaction")
// 	sigHashes, _ := n.MustExecute(func(send func(*messaging.Envelope)) {
// 		send(acctesting.NewTransaction().
// 			WithTimestampVar(&globalNonce).
// 			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
// 			WithTxnHash(ids[0][:]).
// 			Sign(protocol.SignatureTypeED25519, key1.Bytes()).
// 			Build())
// 	})
// 	n.MustWaitForTxns(convertIds32(sigHashes...)...)

// 	txnResp = n.queryTransaction(fmt.Sprintf("foo?txid=%X", ids[0]))
// 	require.False(t, txnResp.Status.Delivered(), "Transaction is was delivered")
// 	require.True(t, txnResp.Status.Pending(), "Transaction is not pending")

// 	t.Log("Signing with key 2 should complete the transaction")
// 	sigHashes, _ = n.MustExecute(func(send func(*messaging.Envelope)) {
// 		send(acctesting.NewTransaction().
// 			WithTimestampVar(&globalNonce).
// 			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
// 			WithTxnHash(ids[0][:]).
// 			Sign(protocol.SignatureTypeED25519, key2.Bytes()).
// 			Build())
// 	})
// 	n.MustWaitForTxns(convertIds32(sigHashes...)...)

// 	txnResp = n.queryTransaction(fmt.Sprintf("foo?txid=%X", ids[0]))
// 	require.True(t, txnResp.Status.Delivered(), "Transaction is was not delivered")
// 	require.False(t, txnResp.Status.Pending(), "Transaction is still pending")

// 	// this should fail, so tell fake tendermint not to give up
// 	// an error will be displayed on the console, but this is exactly what we expect so don't panic
// 	check.Disable = true
// 	t.Run("Signing a complete transaction should fail", func(t *testing.T) {
// 		t.Skip("No longer an error")

// 		_, _, err := n.Execute(func(send func(*messaging.Envelope)) {
// 			send(acctesting.NewTransaction().
// 				WithTimestampVar(&globalNonce).
// 				WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
// 				WithTxnHash(ids[0][:]).
// 				Sign(protocol.SignatureTypeED25519, key2.Bytes()).
// 				Build())
// 		})
// 		require.Error(t, err)
// 	})
// }

func TestAccountAuth(t *testing.T) {
	check := newDefaultCheckError(t, true)
	n := simulator.NewFakeNode(t, check.ErrorHandler())

	fooKey, barKey := generateKey(), generateKey()
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
		require.NoError(t, acctesting.CreateTokenAccount(batch, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
		require.NoError(t, acctesting.CreateSubADI(batch, "foo", "foo/bar"))
		require.NoError(t, acctesting.CreateTokenAccount(batch, "foo/bar/tokens", protocol.AcmeUrl().String(), 0, false))
		require.NoError(t, acctesting.CreateKeyBook(batch, "foo/bar/book", barKey.PubKey().Bytes()))
		require.NoError(t, acctesting.AddCredits(batch, protocol.AccountUrl("foo", "bar", "book", "1"), 1e9))
	})

	// Disable auth
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
			WithBody(&protocol.UpdateAccountAuth{
				Operations: []protocol.AccountAuthOperation{
					&protocol.DisableAccountAuthOperation{
						Authority: protocol.AccountUrl("foo", "book0"),
					},
				},
			}).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	// An unauthorized signer must not be allowed to enable auth
	check.Disable = true
	_, _, err := n.Execute(func(send func(*messaging.Envelope)) {
		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "bar", "book", "1"), 1).
			WithBody(&protocol.UpdateAccountAuth{
				Operations: []protocol.AccountAuthOperation{
					&protocol.EnableAccountAuthOperation{
						Authority: protocol.AccountUrl("foo", "book0"),
					},
				},
			}).
			Initiate(protocol.SignatureTypeLegacyED25519, barKey).
			Build())
	})
	require.Error(t, err, "An unauthorized signer should not be able to enable auth")

	// An unauthorized signer should be able to send tokens
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		exch := new(protocol.SendTokens)
		exch.AddRecipient(protocol.AccountUrl("foo", "bar", "tokens"), big.NewInt(int64(68)))

		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "bar", "book", "1"), 1).
			WithBody(exch).
			Initiate(protocol.SignatureTypeLegacyED25519, barKey).
			Build())
	})

	require.Equal(t, int64(protocol.AcmePrecision-68), n.GetTokenAccount("foo/tokens").Balance.Int64())
	require.Equal(t, int64(68), n.GetTokenAccount("foo/bar/tokens").Balance.Int64())

	// Enable auth
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "book0", "1"), 1).
			WithBody(&protocol.UpdateAccountAuth{
				Operations: []protocol.AccountAuthOperation{
					&protocol.EnableAccountAuthOperation{
						Authority: protocol.AccountUrl("foo", "book0"),
					},
				},
			}).
			Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
			Build())
	})

	// An unauthorized signer should no longer be able to send tokens
	check.Disable = true
	_, _, err = n.Execute(func(send func(*messaging.Envelope)) {
		exch := new(protocol.SendTokens)
		exch.AddRecipient(protocol.AccountUrl("foo", "bar", "tokens"), big.NewInt(int64(68)))

		send(newTxn("foo/tokens").
			WithSigner(protocol.AccountUrl("foo", "bar", "book", "1"), 1).
			WithBody(exch).
			Initiate(protocol.SignatureTypeLegacyED25519, barKey).
			Build())
	})
	require.Error(t, err, "expected a failure but instead an unauthorized signature succeeded")
}

func TestDelegatedKeypageUpdate(t *testing.T) {
	check := newDefaultCheckError(t, false)
	n := simulator.NewFakeNode(t, check.ErrorHandler())
	aliceKey, charlieKey, bobKey, jjkey, newKey1, newKey2 := generateKey(), generateKey(), generateKey(), generateKey(), generateKey(), generateKey()
	charliekeyHash := sha256.Sum256(charlieKey.PubKey().Address())
	newKey1hash := sha256.Sum256(newKey1.PubKey().Address())
	newKey2hash := sha256.Sum256(newKey2.PubKey().Address())

	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, aliceKey, "alice", 1e9))
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, bobKey, "bob", 1e9))
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, charlieKey, "charlie", 1e9))
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, jjkey, "jj", 1e9))
		require.NoError(t, acctesting.UpdateKeyPage(batch, protocol.AccountUrl("alice", "book0", "1"), func(kp *protocol.KeyPage) {
			kp.AddKeySpec(&protocol.KeySpec{Delegate: protocol.AccountUrl("bob", "book0")})
			kp.AddKeySpec(&protocol.KeySpec{Delegate: protocol.AccountUrl("charlie", "book0")})
			kp.AddKeySpec(&protocol.KeySpec{PublicKeyHash: charliekeyHash[:]})
		}))
		require.NoError(t, acctesting.UpdateKeyPage(batch, protocol.AccountUrl("bob", "book0", "1"), func(kp *protocol.KeyPage) {
			kp.AddKeySpec(&protocol.KeySpec{Delegate: protocol.AccountUrl("charlie", "book0")})
		}))
	})

	page := n.GetKeyPage("jj/book0/1")
	//look for the key.
	_, _, found := page.EntryByKeyHash(newKey1hash[:])
	require.False(t, found, "key not found in page")
	//Test with single level Key
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.UpdateKey)
		body.NewKeyHash = newKey1hash[:]
		send(newTxn("jj/book0/1").WithSigner(protocol.AccountUrl("jj", "book0", "1"), 1).WithBody(body).
			Initiate(protocol.SignatureTypeLegacyED25519, jjkey).
			Build())
	})
	page = n.GetKeyPage("jj/book0/1")
	//look for the key.
	_, _, found = page.EntryByKeyHash(newKey1hash[:])
	require.True(t, found, "key not found in page")

	page = n.GetKeyPage("alice/book0/1")
	//look for the key.
	_, _, found = page.EntryByKeyHash(newKey2hash[:])
	require.False(t, found, "key not found in page")

	//Test with singleLevel Delegation
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {

		body := new(protocol.UpdateKey)
		body.NewKeyHash = newKey2hash[:]
		send(newTxn("alice/book0/1").
			WithBody(body).

			// Sign with Charlie via Alice (one-layer delegation)
			WithSigner(protocol.AccountUrl("charlie", "book0", "1"), 1).
			WithDelegator(protocol.AccountUrl("alice", "book0", "1")).
			Initiate(protocol.SignatureTypeED25519, charlieKey).
			Build())
	})

	page = n.GetKeyPage("alice/book0/1")
	//look for the key.
	_, _, found = page.EntryByKeyHash(newKey2hash[:])
	require.True(t, found, "key not found in page")
}

func TestMultiLevelDelegation(t *testing.T) {
	check := newDefaultCheckError(t, false)
	n := simulator.NewFakeNode(t, check.ErrorHandler())
	aliceKey, charlieKey, bobKey := generateKey(), generateKey(), generateKey()
	charliekeyHash := sha256.Sum256(charlieKey.PubKey().Address())
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, aliceKey, "alice", 1e9))
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, bobKey, "bob", 1e9))
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, charlieKey, "charlie", 1e9))
		require.NoError(t, acctesting.UpdateKeyPage(batch, protocol.AccountUrl("alice", "book0", "1"), func(kp *protocol.KeyPage) {
			kp.AddKeySpec(&protocol.KeySpec{Delegate: protocol.AccountUrl("bob", "book0")})
			kp.AddKeySpec(&protocol.KeySpec{Delegate: protocol.AccountUrl("charlie", "book0")})
			kp.AddKeySpec(&protocol.KeySpec{PublicKeyHash: charliekeyHash[:]})
			require.NoError(t, kp.SetThreshold(3))
		}))
		require.NoError(t, acctesting.UpdateKeyPage(batch, protocol.AccountUrl("bob", "book0", "1"), func(kp *protocol.KeyPage) {
			kp.AddKeySpec(&protocol.KeySpec{Delegate: protocol.AccountUrl("charlie", "book0")})
		}))
	})

	_, _, err := n.Execute(func(send func(*messaging.Envelope)) {
		cda := new(protocol.CreateDataAccount)
		cda.Url = protocol.AccountUrl("alice", "data")

		env := newTxn("alice").
			WithBody(cda).

			// Initiate with Alice
			WithSigner(protocol.AccountUrl("alice", "book0", "1"), 1).
			Initiate(protocol.SignatureTypeED25519, aliceKey).

			// Sign with Charlie via Alice (one-layer delegation)
			WithSigner(protocol.AccountUrl("charlie", "book0", "1"), 1).
			WithDelegator(protocol.AccountUrl("alice", "book0", "1")).
			Sign(protocol.SignatureTypeED25519, charlieKey).
			Build()

		// Take Charlie's signature, extract the key signature, and reconstruct
		// it as via Bob via Alice (two-layer delegation)
		sig := env.Signatures[1].(*protocol.DelegatedSignature).Signature
		sig = &protocol.DelegatedSignature{Delegator: protocol.AccountUrl("bob", "book0", "1"), Signature: sig}
		sig = &protocol.DelegatedSignature{Delegator: protocol.AccountUrl("alice", "book0", "1"), Signature: sig}
		env.Signatures = append(env.Signatures, sig)

		send(env)
	})
	require.EqualError(t, err, "signature 2: invalid signature")
}

func TestDuplicateKeyNewKeypage(t *testing.T) {
	check := newDefaultCheckError(t, false)
	n := simulator.NewFakeNode(t, check.ErrorHandler())
	aliceKey := generateKey()
	aliceKeyHash := sha256.Sum256(aliceKey.PubKey().Bytes())
	aliceKey1 := generateKey()
	aliceKeyHash1 := sha256.Sum256(aliceKey1.PubKey().Bytes())
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, aliceKey, "alice", 1e9))
	})

	page := n.GetKeyPage("alice/book0/1")
	require.Len(t, page.Keys, 1)

	key := page.Keys[0]
	require.Equal(t, uint64(0), key.LastUsedOn)
	require.Equal(t, aliceKeyHash[:], key.PublicKeyHash)

	//check for unique keys
	_, txns, err := n.Execute(func(send func(*messaging.Envelope)) {
		cms := new(protocol.CreateKeyPage)
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			KeyHash: aliceKeyHash[:],
		})
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			KeyHash: aliceKeyHash1[:],
		})

		send(newTxn("alice/book0").
			WithSigner(protocol.AccountUrl("alice", "book0", "1"), 1).
			WithBody(cms).
			Initiate(protocol.SignatureTypeLegacyED25519, aliceKey).
			Build())
	})
	require.NoError(t, err)
	_ = txns
	// n.MustWaitForTxns(txns[0][:])
	page = n.GetKeyPage("alice/book0/2")

	require.Len(t, page.Keys, 2)
	_, key1, alice1Found := page.EntryByKeyHash(aliceKeyHash[:])
	_, key2, alice2Found := page.EntryByKeyHash(aliceKeyHash1[:])
	require.True(t, alice1Found, "alice key 1 not found")
	require.Equal(t, uint64(0), key1.GetLastUsedOn())
	require.True(t, alice2Found, "alice key 2 not found")
	require.Equal(t, uint64(0), key2.GetLastUsedOn())

	//check for duplicate keys
	_, _, err = n.Execute(func(send func(*messaging.Envelope)) {
		cms := new(protocol.CreateKeyPage)
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			KeyHash: aliceKeyHash[:],
		})
		cms.Keys = append(cms.Keys, &protocol.KeySpecParams{
			KeyHash: aliceKeyHash[:],
		})

		send(newTxn("alice/book0").
			WithSigner(protocol.AccountUrl("alice", "book0", "1"), 1).
			WithBody(cms).
			Initiate(protocol.SignatureTypeLegacyED25519, aliceKey).
			Build())
	})
	require.EqualError(t, err, "duplicate keys: signing keys of a keypage must be unique")
}
