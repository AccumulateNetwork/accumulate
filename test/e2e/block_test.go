package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func init() { acctesting.EnableDebugFeatures() }

func delivered(status *protocol.TransactionStatus) bool {
	return status.Delivered
}

func TestSendTokensToBadRecipient(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	batch := sim.SubnetFor(aliceUrl).Database.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(alice), protocol.AcmeFaucetAmount, 1e9))
	require.NoError(t, batch.Commit())

	exch := new(protocol.SendTokens)
	exch.AddRecipient(acctesting.MustParseUrl("foo"), big.NewInt(int64(1000)))
	env := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithTimestampVar(&timestamp).
		WithSigner(aliceUrl.RootIdentity(), 1).
		WithBody(exch).
		Initiate(protocol.SignatureTypeLegacyED25519, alice).
		Build()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	// The balance should be unchanged
	batch = sim.SubnetFor(aliceUrl).Database.Begin(false)
	defer batch.Discard()
	var account *protocol.LiteTokenAccount
	require.NoError(t, batch.Account(aliceUrl).GetStateAs(&account))
	assert.Equal(t, int64(protocol.AcmeFaucetAmount*protocol.AcmePrecision), account.Balance.Int64())

	// The synthetic transaction should fail
	synth, err := batch.Transaction(env.Transaction[0].GetHash()).GetSyntheticTxns()
	require.NoError(t, err)
	batch = sim.SubnetFor(url.MustParse("foo")).Database.Begin(false)
	defer batch.Discard()
	status, err := batch.Transaction(synth.Hashes[0][:]).GetStatus()
	require.NoError(t, err)
	assert.Equal(t, protocol.ErrorCodeNotFound.GetEnumValue(), status.Code)
}

func TestSendTokensToBadRecipient2(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey("Bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
	batch := sim.SubnetFor(aliceUrl).Database.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(alice), protocol.AcmeFaucetAmount, 1e9))
	require.NoError(t, batch.Commit())

	var creditsBefore uint64
	_ = sim.SubnetFor(aliceUrl).Database.View(func(batch *database.Batch) error {
		var account *protocol.LiteIdentity
		require.NoError(t, batch.Account(aliceUrl.RootIdentity()).GetStateAs(&account))
		creditsBefore = account.CreditBalance
		return nil
	})

	exch := new(protocol.SendTokens)
	exch.AddRecipient(acctesting.MustParseUrl("foo"), big.NewInt(int64(1000)))
	exch.AddRecipient(bobUrl, big.NewInt(int64(1000)))
	env := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithTimestampVar(&timestamp).
		WithSigner(aliceUrl, 1).
		WithBody(exch).
		Initiate(protocol.SignatureTypeLegacyED25519, alice).
		Build()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	var creditsAfter uint64
	_ = sim.SubnetFor(aliceUrl).Database.View(func(batch *database.Batch) error {
		var account *protocol.LiteIdentity
		require.NoError(t, batch.Account(aliceUrl.RootIdentity()).GetStateAs(&account))
		creditsAfter = account.CreditBalance
		return nil
	})

	expectedFee := FeeSendTokens - (FeeSendTokens-FeeFailedMaximum)/2
	require.Equal(t, creditsBefore-expectedFee.AsUInt64(), creditsAfter)
}

func TestCreateRootIdentity(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	lite := acctesting.GenerateKey(t.Name(), "Lite")
	liteUrl := acctesting.AcmeLiteAddressStdPriv(lite)
	batch := sim.SubnetFor(liteUrl).Database.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(lite), protocol.AcmeFaucetAmount, 1e9))
	require.NoError(t, batch.Commit())

	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), alice)
	keyHash := sha256.Sum256(aliceKey[32:])

	_, txn := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl.RootIdentity()).
			WithTimestampVar(&timestamp).
			WithSigner(liteUrl.RootIdentity(), 1).
			WithBody(&protocol.CreateIdentity{
				Url:        alice,
				KeyHash:    keyHash[:],
				KeyBookUrl: alice.JoinPath("book"),
			}).
			Initiate(protocol.SignatureTypeLegacyED25519, lite).
			Build(),
	)...)

	// There should be a synthetic transaction
	require.Len(t, txn, 2)
	require.IsType(t, (*SyntheticCreateIdentity)(nil), txn[1].Body)

	// Verify the account is created
	_ = sim.SubnetFor(alice).Database.View(func(batch *database.Batch) error {
		var identity *protocol.ADI
		require.NoError(t, batch.Account(alice).GetStateAs(&identity))
		return nil
	})
}

func TestWriteToLiteDataAccount(t *testing.T) {
	// Setup
	alice := acctesting.GenerateKey(t.Name())
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(tmed25519.PrivKey(alice))
	aliceAdi := url.MustParse("alice")

	firstEntry := DataEntry{}
	firstEntry.Data = append(firstEntry.Data, []byte{})
	firstEntry.Data = append(firstEntry.Data, []byte("Factom PRO"))
	firstEntry.Data = append(firstEntry.Data, []byte("Tutorial"))
	chainId := ComputeLiteDataAccountId(&firstEntry)
	liteDataAddress, err := LiteDataAddress(chainId)
	require.NoError(t, err)

	t.Run("Lite", func(t *testing.T) {
		// Initialize
		var timestamp uint64
		sim := simulator.New(t, 3)
		sim.InitChain()

		batch := sim.SubnetFor(aliceUrl).Database.Begin(true)
		defer batch.Discard()
		require.NoError(sim, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(alice), 1e9, 1e9))
		require.NoError(sim, batch.Commit())

		// Write data
		env := acctesting.NewTransaction().
			WithPrincipal(liteDataAddress).
			WithBody(&WriteData{Entry: firstEntry}).
			WithSigner(aliceUrl, 1).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, alice).
			Build()
		sim.MustSubmitAndExecuteBlock(env)
		status, _ := sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

		// Verify
		batch = sim.SubnetFor(liteDataAddress).Database.Begin(false)
		defer batch.Discard()
		verifyLiteDataAccount(t, batch, &firstEntry, status[0])
	})

	t.Run("ADI", func(t *testing.T) {
		// Initialize
		var timestamp uint64
		sim := simulator.New(t, 3)
		sim.InitChain()

		batch := sim.SubnetFor(aliceAdi).Database.Begin(true)
		defer batch.Discard()
		require.NoError(sim, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(alice), "alice", 1e9))
		require.NoError(sim, batch.Commit())

		// Write data
		env := acctesting.NewTransaction().
			WithPrincipal(liteDataAddress).
			WithBody(&WriteData{Entry: firstEntry}).
			WithSigner(aliceAdi.JoinPath("book0", "1"), 1).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, alice).
			Build()
		sim.MustSubmitAndExecuteBlock(env)
		status, _ := sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

		// Verify
		batch = sim.SubnetFor(liteDataAddress).Database.Begin(false)
		defer batch.Discard()
		verifyLiteDataAccount(t, batch, &firstEntry, status[0])
	})
}

func verifyLiteDataAccount(t *testing.T, batch *database.Batch, firstEntry *DataEntry, status *TransactionStatus) {
	chainId := ComputeLiteDataAccountId(firstEntry)
	liteDataAddress, err := LiteDataAddress(chainId)
	require.NoError(t, err)

	partialChainId, err := ParseLiteDataAddress(liteDataAddress)
	require.NoError(t, err)
	var account *LiteDataAccount
	require.NoError(t, batch.Account(liteDataAddress).GetStateAs(&account))
	require.Equal(t, liteDataAddress.String(), account.Url.String())
	require.Equal(t, append(partialChainId, account.Tail...), chainId)

	firstEntryHash, err := ComputeLiteEntryHashFromEntry(chainId, firstEntry)
	require.NoError(t, err)

	// Verify the entry hash in the transaction result
	require.IsType(t, (*WriteDataResult)(nil), status.Result)
	txResult := status.Result.(*WriteDataResult)
	require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(txResult.EntryHash[:]), "Transaction result entry hash does not match")

	// Verify the entry hash returned by Entry
	dataChain, err := batch.Account(liteDataAddress).Data()
	require.NoError(t, err)
	entry, err := dataChain.Entry(0)
	require.NoError(t, err)
	hashFromEntry, err := ComputeLiteEntryHashFromEntry(chainId, entry)
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(hashFromEntry), "Chain Entry.Hash does not match")
	//sample verification for calculating the hash from lite data entry
	hashes, err := dataChain.GetHashes(0, 1)
	require.NoError(t, err)
	ent, err := dataChain.Entry(0)
	require.NoError(t, err)
	id := ComputeLiteDataAccountId(ent)
	newh, err := ComputeLiteEntryHashFromEntry(id, ent)
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(hashes[0]), "Chain GetHashes does not match")
	require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(newh), "Chain GetHashes does not match")
}

func TestCreateSubIdentityWithLite(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	liteKey := acctesting.GenerateKey(t.Name(), "Lite")
	liteUrl := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), "Alice")
	keyHash := sha256.Sum256(aliceKey[32:])
	sim.CreateIdentity(alice, aliceKey[32:])
	sim.CreateAccount(&LiteIdentity{Url: liteUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: liteUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9)})

	_, err := sim.SubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl).
			WithTimestampVar(&timestamp).
			WithSigner(liteUrl, 1).
			WithBody(&protocol.CreateIdentity{
				Url:        alice.JoinPath("sub"),
				KeyHash:    keyHash[:],
				KeyBookUrl: alice.JoinPath("sub", "book"),
			}).
			Initiate(protocol.SignatureTypeLegacyED25519, liteKey).
			Build(),
	)
	var err2 *errors.Error
	require.Error(t, err)
	require.ErrorAs(t, err, &err2)
	require.Equal(t, errors.StatusUnauthorized, err2.Code)
}

func TestCreateIdentityWithRemoteLite(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	liteKey := acctesting.GenerateKey(t.Name(), "Lite")
	liteUrl := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), "Alice")
	keyHash := sha256.Sum256(aliceKey[32:])
	sim.CreateAccount(&LiteIdentity{Url: liteUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: liteUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9)})

	_, txn := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice).
			WithTimestampVar(&timestamp).
			WithSigner(liteUrl, 1).
			WithBody(&protocol.CreateIdentity{
				Url:        alice,
				KeyHash:    keyHash[:],
				KeyBookUrl: alice.JoinPath("book"),
			}).
			Initiate(protocol.SignatureTypeLegacyED25519, liteKey).
			Build(),
	)...)

	// There should not be a synthetic transaction
	require.Len(t, txn, 1)

	// Verify the account is created
	_ = sim.SubnetFor(alice).Database.View(func(batch *database.Batch) error {
		var identity *protocol.ADI
		require.NoError(t, batch.Account(alice).GetStateAs(&identity))
		return nil
	})
}

func TestGetBlocks(t *testing.T) {
	client, err := client.New("https://testnet.accumulatenetwork.io")
	require.NoError(t, err)

	// Query DN blocks
	req := new(api.MinorBlocksQuery)
	req.Url = url.MustParse("dn")
	req.Start = 100
	req.Count = 10
	req.TxFetchMode = query.TxFetchModeExpand
	resp, err := client.QueryMinorBlocks(context.Background(), req)
	require.NoError(t, err)

	// For each DN block
	for _, block := range resp.Items {
		// This block would be unnecessary for JavaScript
		data, err := json.Marshal(block)
		require.NoError(t, err)
		var resp api.MinorQueryResponse
		require.NoError(t, json.Unmarshal(data, &resp))

		// Just to make the output cleaner for this test
		if len(resp.Transactions) == 0 {
			continue
		}

		fmt.Printf("Block %d @ %v\n", resp.BlockIndex, resp.BlockTime)

		// Print each transaction and record anchors
		var anchors []*protocol.SyntheticAnchor
		for _, env := range resp.Transactions {
			fmt.Printf("    %v %X (%v)", env.Transaction.Header.Principal, env.Transaction.GetHash()[:4], env.Transaction.Body.Type())

			if anchor, ok := env.Transaction.Body.(*protocol.SyntheticAnchor); ok {
				anchors = append(anchors, anchor)
				fmt.Printf(" block %d from %v", anchor.Block, anchor.Source)
			}
			fmt.Println()
		}

		// For each anchor in the block
		for _, anchor := range anchors {
			// Query the corresponding BVN block
			req := new(api.MinorBlocksQuery)
			req.Url = anchor.Source
			req.Start = anchor.Block
			req.Count = 1
			req.TxFetchMode = query.TxFetchModeExpand
			resp, err := client.QueryMinorBlocks(context.Background(), req)
			require.NoError(t, err)

			// Print out the transactions
			for _, block := range resp.Items {
				data, err := json.Marshal(block)
				require.NoError(t, err)
				var resp api.MinorQueryResponse
				require.NoError(t, json.Unmarshal(data, &resp))
				for _, env := range resp.Transactions {
					fmt.Printf("    %v %X (%v)\n", env.Transaction.Header.Principal, env.Transaction.GetHash()[:4], env.Transaction.Body.Type())
				}
			}
		}
	}
}
