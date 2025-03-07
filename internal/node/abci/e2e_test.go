// Copyright 2024 The Accumulate Authors
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

	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
	randpkg "golang.org/x/exp/rand"
)

var rand = randpkg.New(randpkg.NewSource(0))

type Tx = messaging.Envelope

func init() { acctesting.EnableDebugFeatures() }

func TestCreateLiteAccount(t *testing.T) {
	n := simulator.NewFakeNodeV1(t, nil)

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

		send(
			MustBuild(n.T(), build.Transaction().
				For(protocol.FaucetUrl.RootIdentity()).
				Body(body).
				SignWith(protocol.FaucetUrl).Version(1).Timestamp(time.Now().

				//acme to credits @ $0.05 acme price is 1:5
				UnixNano()).Signer(protocol.Faucet.Signer())))
	})

	n.Update(func(batch *database.Batch) {

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
				send(
					MustBuild(n.T(), build.Transaction().
						For(mustParseOrigin(senderUrl.String())).
						Body(exch).
						SignWith(mustParseOrigin(senderUrl.String())).Version(1).Timestamp(&globalNonce).PrivateKey(sender).Type(protocol.SignatureTypeLegacyED25519)),
				)
			}
		})
	}

	return senderUrl.String(), balance
}

func TestFaucet(t *testing.T) {
	n := simulator.NewFakeNodeV1(t, nil)

	alice := generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.AcmeFaucet)
		body.Url = aliceUrl

		send(
			MustBuild(t, build.Transaction().
				For(protocol.FaucetUrl).
				Body(body).
				SignWith(protocol.FaucetUrl).Version(1).Timestamp(time.Now().UnixNano()).Signer(protocol.Faucet.Signer())))
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
		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin(sponsorUrl)).
				Body(adi).
				SignWith(mustParseOrigin(sponsorUrl)).Version(1).Timestamp(&globalNonce).PrivateKey(liteAccount).Type(protocol.SignatureTypeLegacyED25519)),
		)
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
	// require.NoError(t, ledger.Main().GetAs(&ledgerState))
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
		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin(sponsorUrl)).
				Body(adi).
				SignWith(mustParseOrigin(sponsorUrl)).Version(1).Timestamp(&globalNonce).PrivateKey(liteAccount).Type(protocol.SignatureTypeLegacyED25519)),
		)
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
		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin(sponsorUrl)).
				Body(adi).
				SignWith(mustParseOrigin(sponsorUrl)).Version(1).Timestamp(&globalNonce).PrivateKey(liteAccount).Type(protocol.SignatureTypeLegacyED25519)),
		)
	})

	res := n.QueryTx(txn[0][:], time.Second, true)
	h := res.Produced.Records[0].Value.Hash()
	res = n.QueryTx(h[:], time.Second, true)
	require.Equal(t, errors.BadUrlLength, res.Status)
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
		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin(sponsorUrl)).
				Body(adi).
				SignWith(mustParseOrigin(sponsorUrl)).Version(1).Timestamp(&globalNonce).PrivateKey(liteAccount).Type(protocol.SignatureTypeLegacyED25519)),
		)
	})
	require.Error(t, err)
}

func TestCreateLiteDataAccount(t *testing.T) {

	//this test exercises WriteDataTo and SyntheticWriteData validators

	firstEntry := protocol.DoubleHashDataEntry{}

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
		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("FooBar")).
				Body(wdt).
				SignWith(protocol.AccountUrl("FooBar", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(adiKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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
		synthIds, err := batch.Transaction(ids[0][:]).Produced().Get()
		require.NoError(t, err)

		// Verify the entry hash in the transaction result
		h := synthIds[0].Hash()
		txStatus, err := batch.Transaction(h[:]).Status().Get()
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
			send(
				MustBuild(t, build.Transaction().
					For(mustParseOrigin("FooBar")).
					Body(tac).
					SignWith(protocol.AccountUrl("FooBar", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(adiKey).Type(protocol.SignatureTypeLegacyED25519)),
			)
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
			send(
				MustBuild(t, build.Transaction().
					For(mustParseOrigin("FooBar")).
					Body(cda).
					SignWith(protocol.AccountUrl("FooBar", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(adiKey).Type(protocol.SignatureTypeLegacyED25519).
					SignWith(protocol.AccountUrl("FooBar", "foo", "book1", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(pageKey).Type(protocol.SignatureTypeED25519).
					SignWith(protocol.AccountUrl("FooBar", "mgr", "book1", "2")).Version(1).Timestamp(&globalNonce).PrivateKey(pageKey).Type(protocol.SignatureTypeED25519)),
			)
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
			send(
				MustBuild(t, build.Transaction().
					For(mustParseOrigin("FooBar")).
					Body(tac).
					SignWith(protocol.AccountUrl("FooBar", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(adiKey).Type(protocol.SignatureTypeLegacyED25519)),
			)
		})

		r := n.GetDataAccount("FooBar/oof")
		require.Equal(t, "acc://FooBar.acme/oof", r.Url.String())
		require.Contains(t, n.GetDirectory("FooBar"), protocol.AccountUrl("FooBar", "oof").String())

		wd := new(protocol.WriteData)
		n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
			entry := new(protocol.DoubleHashDataEntry)
			wd.Entry = entry
			entry.Data = append(entry.Data, []byte("thequickbrownfoxjumpsoverthelazydog"))
			for i := 0; i < 10; i++ {
				entry.Data = append(entry.Data, []byte(fmt.Sprintf("test id %d", i)))
			}

			send(
				MustBuild(t, build.Transaction().
					For(mustParseOrigin("FooBar/oof")).
					Body(wd).
					SignWith(protocol.AccountUrl("FooBar", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(adiKey).Type(protocol.SignatureTypeLegacyED25519)),
			)
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
			send(
				MustBuild(t, build.Transaction().
					For(mustParseOrigin("FooBar")).
					Body(tac).
					SignWith(protocol.AccountUrl("FooBar", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(adiKey).Type(protocol.SignatureTypeLegacyED25519)),
			)
		})

		r := n.GetDataAccount("FooBar/scr")
		require.Equal(t, "acc://FooBar.acme/scr", r.Url.String())
		require.Contains(t, n.GetDirectory("FooBar"), protocol.AccountUrl("FooBar", "scr").String())

		wd := new(protocol.WriteData)
		wd.Scratch = true
		n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
			entry := new(protocol.DoubleHashDataEntry)
			wd.Entry = entry
			entry.Data = append(entry.Data, []byte("thequickbrownfoxjumpsoverthelazydog"))
			for i := 0; i < 10; i++ {
				entry.Data = append(entry.Data, []byte(fmt.Sprintf("test id %d", i)))
			}

			send(
				MustBuild(t, build.Transaction().
					For(mustParseOrigin("FooBar/scr")).
					Body(wd).
					SignWith(protocol.AccountUrl("FooBar", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(adiKey).Type(protocol.SignatureTypeLegacyED25519)),
			)
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
			send(
				MustBuild(t, build.Transaction().
					For(mustParseOrigin("FooBar")).
					Body(tac).
					SignWith(protocol.AccountUrl("FooBar", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(adiKey).Type(protocol.SignatureTypeLegacyED25519)),
			)
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
			send(
				MustBuild(t, build.Transaction().
					For(mustParseOrigin("FooBar")).
					Body(tac).
					SignWith(protocol.AccountUrl("FooBar", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(adiKey).Type(protocol.SignatureTypeLegacyED25519).
					SignWith(protocol.AccountUrl("FooBar", "book1", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(pageKey).Type(protocol.SignatureTypeED25519)),
			)
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
			send(
				MustBuild(t, build.Transaction().
					For(mustParseOrigin("alice")).
					Body(tac).
					SignWith(protocol.AccountUrl("alice", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(aliceKey).Type(protocol.SignatureTypeLegacyED25519).
					SignWith(protocol.AccountUrl("bob", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(bobKey).Type(protocol.SignatureTypeED25519)),
			)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin(aliceUrl.String())).
				Body(exch).
				SignWith(aliceUrl.RootIdentity()).Version(1).Timestamp(&globalNonce).PrivateKey(alice).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/tokens")).
				Body(exch).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin(aliceUrl.String())).
				Body(exch).
				SignWith(aliceUrl.RootIdentity()).Version(1).Timestamp(&globalNonce).PrivateKey(alice).Type(protocol.SignatureTypeLegacyED25519)),
		)
	})

	// The synthetic transaction should fail
	res := n.QueryTx(txnHashes[0][:], time.Second, true)
	h := res.Produced.Records[0].Value.Hash()
	res = n.QueryTx(h[:], time.Second, true)
	require.Equal(t, errors.NotFound, res.Status)

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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/book0")).
				Body(cms).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo")).
				Body(csg).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/book1")).
				Body(cms).
				SignWith(protocol.AccountUrl("foo", "book1", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(testKey1).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/book1/1")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book1", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(testKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/book1/1")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book1", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(testKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/book1/1")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book1", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(testKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
	})
	r := n.QueryTx(txnHashes[0][:], 0, false)
	require.False(t, r.Status == errors.Pending, "Transaction is still pending")

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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/book1/1")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book1", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(testKey1).Type(protocol.SignatureTypeLegacyED25519)),
		)
	})
	h1 := sha256.Sum256(testKey1.PubKey().Bytes())
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		op := new(protocol.RemoveKeyOperation)

		op.Entry.KeyHash = h1[:]
		body := new(protocol.UpdateKeyPage)
		body.Operation = append(body.Operation, op)

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/book1/1")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book1", "1")).Version(2).Timestamp(&globalNonce).PrivateKey(testKey2).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin(liteUrl.RootIdentity().String())).
				Body(adi).
				SignWith(mustParseOrigin(liteUrl.RootIdentity().String())).Version(1).Timestamp(&globalNonce).PrivateKey(liteKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
	})

	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.AddCredits(batch, keyPageUrl, 1e9))
	})

	keyPageHeight := getHeight(keyPageUrl)

	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		tac := new(protocol.CreateTokenAccount)
		tac.Url = tokenUrl
		tac.TokenUrl = protocol.AcmeUrl()
		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo")).
				Body(tac).
				SignWith(protocol.FormatKeyPageUrl(keyBookUrl, 0)).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/tokens")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/tokens")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
	})

	//test to make sure supply limit is set
	issuer := n.GetTokenIssuer("foo/tokens")
	require.Equal(t, supplyLimit.Int64(), issuer.SupplyLimit.Int64())

	//issue tokens successfully
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.IssueTokens)
		body.Recipient = liteAddr
		body.Amount.SetUint64(123)

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/tokens")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/tokens")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

	n := simulator.NewFakeNodeV1(t, check.ErrorHandler())

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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/tokens")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/tokens")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("foo/tokens")).
				Body(body).
				SignWith(protocol.AccountUrl("foo", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(fooKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin(liteAcmeAddr.String())).
				Body(body).
				SignWith(liteId.RootIdentity()).Version(1).Timestamp(&globalNonce).PrivateKey(liteKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
	})

	//now lets burn some tokens to see if they get returned to the supply
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.BurnTokens)
		//burn the underLimit amount to see if that gets returned to the pool
		body.Amount.SetInt64(underLimit)

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin(liteAddr.String())).
				Body(body).
				SignWith(liteAddr.RootIdentity()).Version(1).Timestamp(&globalNonce).PrivateKey(liteKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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
	state, err := account.Main().Get()
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
			var txState *messaging.TransactionMessage
			err := batch.Message2(id).Main().GetAs(&txState)
			require.NoError(t, err)
			txStatus, err := batch.Transaction(id).Status().Get()
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
		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("jj/book0/1")).
				Body(body).
				SignWith(protocol.AccountUrl("jj", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(jjkey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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
		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("alice/book0/1")).
				Body(body).
				SignWith(
					// Sign with Charlie
					protocol.AccountUrl("charlie", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(charlieKey).Type(protocol.SignatureTypeED25519)),
		)
	})

	page = n.GetKeyPage("alice/book0/1")
	//look for the key.
	_, _, found = page.EntryByKeyHash(newKey2hash[:])
	require.True(t, found, "key not found in page")
}

func TestLxrMiningSignature(t *testing.T) {
	// Create a new test node
	check := newDefaultCheckError(t, false)
	n := simulator.NewFakeNode(t, check.ErrorHandler())
	
	// Skip the test if not running in v2 mode
	if !n.IsV2() {
		t.Skip("LXR mining signatures are only supported in v2")
	}
	
	// Create a key for the miner
	minerKey := generateKey()
	minerKeyHash := sha256.Sum256(minerKey.PubKey().Bytes())
	
	// Create an ADI for the miner
	n.Update(func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, minerKey, "miner", 1e9))
	})
	
	// Get the key page
	keyPage := n.GetKeyPage("miner/book0/1")
	require.Len(t, keyPage.Keys, 1)
	require.Equal(t, minerKeyHash[:], keyPage.Keys[0].PublicKeyHash)
	
	// Enable mining on the key page
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.UpdateKeyPage)
		body.Operation = protocol.KeyPageOperation_SetMiningEnabled
		body.MiningEnabled = true
		body.MiningDifficulty = 100 // Set a low difficulty for testing
		
		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("miner/book0/1")).
				Body(body).
				SignWith(protocol.AccountUrl("miner", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(minerKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
	})
	
	// Verify mining is enabled
	keyPage = n.GetKeyPage("miner/book0/1")
	require.True(t, keyPage.MiningEnabled)
	require.Equal(t, uint64(100), keyPage.MiningDifficulty)
	
	// Create a transaction to sign with LXR mining
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AccountUrl("miner")
	txn.Body = new(protocol.WriteData)
	txnHash := txn.GetHash()
	
	// Create a block hash to mine against
	blockHash := [32]byte{}
	copy(blockHash[:], acctesting.GenerateKey("blockhash").Bytes())
	
	// Enable test environment for LXR hasher
	lxr.IsTestEnvironment = true
	
	// Create a hasher for mining
	hasher := lxr.NewHasher()
	
	// Create a nonce
	nonce := []byte("test nonce for mining")
	
	// Calculate the proof of work
	computedHash, difficulty := hasher.CalculatePow(blockHash[:], nonce)
	require.GreaterOrEqual(t, difficulty, uint64(100), "Mining difficulty should be at least 100")
	
	// Create a valid mining signature
	validSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion: keyPage.Version,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Submit the transaction with the LXR mining signature
	txid, err := n.SubmitTxn(txn, validSig)
	require.NoError(t, err, "Failed to submit transaction with LXR mining signature")
	
	// Wait for the transaction to be processed
	n.MustWaitForTxns(txid)
	
	// Verify the transaction was processed successfully
	status, err := n.GetTxnStatus(txid)
	require.NoError(t, err)
	require.Equal(t, protocol.TransactionStatusSuccess, status.Code)
	
	// Try with an invalid signature (wrong block hash)
	invalidBlockHash := [32]byte{}
	copy(invalidBlockHash[:], acctesting.GenerateKey("invalid").Bytes())
	
	invalidSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     invalidBlockHash, // Different block hash
		Signer:        protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion: keyPage.Version,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Submit the transaction with the invalid LXR mining signature
	txn2 := new(protocol.Transaction)
	txn2.Header.Principal = protocol.AccountUrl("miner")
	txn2.Body = new(protocol.WriteData)
	
	_, err = n.SubmitTxn(txn2, invalidSig)
	require.Error(t, err, "Transaction with invalid LXR mining signature should fail")
	require.Contains(t, err.Error(), "invalid LXR mining signature")
	
	// Test with an invalid signature (wrong computed hash)
	invalidComputedHash := [32]byte{}
	copy(invalidComputedHash[:], acctesting.GenerateKey("invalid-hash").Bytes())
	
	invalidSig2 := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  invalidComputedHash, // Different computed hash
		BlockHash:     blockHash,
		Signer:        protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion: keyPage.Version,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Submit the transaction with the invalid computed hash
	txn3 := new(protocol.Transaction)
	txn3.Header.Principal = protocol.AccountUrl("miner")
	txn3.Body = new(protocol.WriteData)
	
	_, err = n.SubmitTxn(txn3, invalidSig2)
	require.Error(t, err, "Transaction with invalid computed hash should fail")
	require.Contains(t, err.Error(), "invalid LXR mining signature")
	
	// Test with a higher difficulty than the signature provides
	// First update the key page to require a higher difficulty
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.UpdateKeyPage)
		body.Operation = protocol.KeyPageOperation_SetMiningEnabled
		body.MiningEnabled = true
		body.MiningDifficulty = difficulty + 1000 // Set a much higher difficulty
		
		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("miner/book0/1")).
				Body(body).
				SignWith(protocol.AccountUrl("miner", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(minerKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
	})
	
	// Verify mining difficulty is updated
	keyPage = n.GetKeyPage("miner/book0/1")
	require.True(t, keyPage.MiningEnabled)
	require.Equal(t, difficulty + 1000, keyPage.MiningDifficulty)
	
	// Try to use the same signature that doesn't meet the new difficulty
	txn4 := new(protocol.Transaction)
	txn4.Header.Principal = protocol.AccountUrl("miner")
	txn4.Body = new(protocol.WriteData)
	
	_, err = n.SubmitTxn(txn4, validSig)
	require.Error(t, err, "Transaction with insufficient mining difficulty should fail")
	require.Contains(t, err.Error(), "difficulty")
	
	// Test with a disabled mining key page
	// Disable mining on the key page
	n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.UpdateKeyPage)
		body.Operation = protocol.KeyPageOperation_SetMiningEnabled
		body.MiningEnabled = false
		
		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("miner/book0/1")).
				Body(body).
				SignWith(protocol.AccountUrl("miner", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(minerKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
	})
	
	// Verify mining is disabled
	keyPage = n.GetKeyPage("miner/book0/1")
	require.False(t, keyPage.MiningEnabled)
	
	// Try to use a mining signature with mining disabled
	txn5 := new(protocol.Transaction)
	txn5.Header.Principal = protocol.AccountUrl("miner")
	txn5.Body = new(protocol.WriteData)
	
	_, err = n.SubmitTxn(txn5, validSig)
	require.Error(t, err, "Transaction with mining disabled should fail")
	require.Contains(t, err.Error(), "mining is not enabled")
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("alice/book0")).
				Body(cms).
				SignWith(protocol.AccountUrl("alice", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(aliceKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
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

		send(
			MustBuild(t, build.Transaction().
				For(mustParseOrigin("alice/book0")).
				Body(cms).
				SignWith(protocol.AccountUrl("alice", "book0", "1")).Version(1).Timestamp(&globalNonce).PrivateKey(aliceKey).Type(protocol.SignatureTypeLegacyED25519)),
		)
	})
	require.EqualError(t, err, "duplicate keys: signing keys of a keypage must be unique")
}
