package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() { acctesting.EnableDebugFeatures() }

func delivered(status *TransactionStatus) bool {
	return status.Delivered
}

func updateAccount[T Account](sim *simulator.Simulator, accountUrl *url.URL, fn func(account T)) {
	sim.UpdateAccount(accountUrl, func(account Account) {
		var typed T
		err := encoding.SetPtr(account, &typed)
		if err != nil {
			sim.Log(err)
			sim.FailNow()
		}

		fn(typed)
	})
}

func TestSendTokensToBadRecipient(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	batch := sim.PartitionFor(aliceUrl).Database.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(alice), AcmeFaucetAmount, 1e9))
	require.NoError(t, batch.Commit())

	exch := new(SendTokens)
	exch.AddRecipient(protocol.AccountUrl("foo"), big.NewInt(int64(1000)))
	env := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithTimestampVar(&timestamp).
		WithSigner(aliceUrl.RootIdentity(), 1).
		WithBody(exch).
		Initiate(SignatureTypeLegacyED25519, alice).
		Build()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	// The balance should be unchanged
	batch = sim.PartitionFor(aliceUrl).Database.Begin(false)
	defer batch.Discard()
	var account *LiteTokenAccount
	require.NoError(t, batch.Account(aliceUrl).GetStateAs(&account))
	assert.Equal(t, int64(AcmeFaucetAmount*AcmePrecision), account.Balance.Int64())

	// The synthetic transaction should fail
	synth, err := batch.Transaction(env.Transaction[0].GetHash()).GetSyntheticTxns()
	require.NoError(t, err)
	batch = sim.PartitionFor(protocol.AccountUrl("foo")).Database.Begin(false)
	defer batch.Discard()
	h := synth.Entries[0].Hash()
	status, err := batch.Transaction(h[:]).GetStatus()
	require.NoError(t, err)
	assert.Equal(t, ErrorCodeNotFound.GetEnumValue(), status.Code)
}

func TestSendTokensToBadRecipient2(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey("Bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
	batch := sim.PartitionFor(aliceUrl).Database.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(alice), AcmeFaucetAmount, 1e9))
	require.NoError(t, batch.Commit())

	var creditsBefore uint64
	_ = sim.PartitionFor(aliceUrl).Database.View(func(batch *database.Batch) error {
		var account *LiteIdentity
		require.NoError(t, batch.Account(aliceUrl.RootIdentity()).GetStateAs(&account))
		creditsBefore = account.CreditBalance
		return nil
	})

	exch := new(SendTokens)
	exch.AddRecipient(protocol.AccountUrl("foo"), big.NewInt(int64(1000)))
	exch.AddRecipient(bobUrl, big.NewInt(int64(1000)))
	env := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithTimestampVar(&timestamp).
		WithSigner(aliceUrl, 1).
		WithBody(exch).
		Initiate(SignatureTypeLegacyED25519, alice).
		Build()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	var creditsAfter uint64
	_ = sim.PartitionFor(aliceUrl).Database.View(func(batch *database.Batch) error {
		var account *LiteIdentity
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
	sim.InitFromGenesis()

	lite := acctesting.GenerateKey(t.Name(), "Lite")
	liteUrl := acctesting.AcmeLiteAddressStdPriv(lite)
	batch := sim.PartitionFor(liteUrl).Database.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(lite), AcmeFaucetAmount, 1e9))
	require.NoError(t, batch.Commit())

	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), alice)
	keyHash := sha256.Sum256(aliceKey[32:])

	_, txn := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl.RootIdentity()).
			WithTimestampVar(&timestamp).
			WithSigner(liteUrl.RootIdentity(), 1).
			WithBody(&CreateIdentity{
				Url:        alice,
				KeyHash:    keyHash[:],
				KeyBookUrl: alice.JoinPath("book"),
			}).
			Initiate(SignatureTypeLegacyED25519, lite).
			Build(),
	)...)

	// There should be a synthetic transaction
	require.Len(t, txn, 2)
	require.IsType(t, (*SyntheticCreateIdentity)(nil), txn[1].Body)

	// Verify the account is created
	_ = sim.PartitionFor(alice).Database.View(func(batch *database.Batch) error {
		var identity *ADI
		require.NoError(t, batch.Account(alice).GetStateAs(&identity))
		return nil
	})
}

func TestWriteToLiteDataAccount(t *testing.T) {
	// Setup
	alice := acctesting.GenerateKey(t.Name())
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(tmed25519.PrivKey(alice))
	aliceAdi := protocol.AccountUrl("alice")

	firstEntry := AccumulateDataEntry{}
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
		sim.InitFromGenesis()

		batch := sim.PartitionFor(aliceUrl).Database.Begin(true)
		defer batch.Discard()
		require.NoError(sim, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(alice), 1e9, 1e9))
		require.NoError(sim, batch.Commit())

		// Write data
		env := acctesting.NewTransaction().
			WithPrincipal(liteDataAddress).
			WithBody(&WriteData{Entry: &firstEntry}).
			WithSigner(aliceUrl, 1).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, alice).
			Build()
		sim.MustSubmitAndExecuteBlock(env)
		status, _ := sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

		// Verify
		batch = sim.PartitionFor(liteDataAddress).Database.Begin(false)
		defer batch.Discard()
		verifyLiteDataAccount(t, batch, &firstEntry, status[0])
	})

	t.Run("ADI", func(t *testing.T) {
		// Initialize
		var timestamp uint64
		sim := simulator.New(t, 3)
		sim.InitFromGenesis()

		batch := sim.PartitionFor(aliceAdi).Database.Begin(true)
		defer batch.Discard()
		require.NoError(sim, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(alice), "alice", 1e9))
		require.NoError(sim, batch.Commit())

		// Write data
		env := acctesting.NewTransaction().
			WithPrincipal(liteDataAddress).
			WithBody(&WriteData{Entry: &firstEntry}).
			WithSigner(aliceAdi.JoinPath("book0", "1"), 1).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, alice).
			Build()
		sim.MustSubmitAndExecuteBlock(env)
		status, _ := sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

		// Verify
		batch = sim.PartitionFor(liteDataAddress).Database.Begin(false)
		defer batch.Discard()
		verifyLiteDataAccount(t, batch, &firstEntry, status[0])
	})
}

func verifyLiteDataAccount(t *testing.T, batch *database.Batch, firstEntry DataEntry, status *TransactionStatus) {
	chainId := ComputeLiteDataAccountId(firstEntry)
	liteDataAddress, err := LiteDataAddress(chainId)
	require.NoError(t, err)

	partialChainId, err := ParseLiteDataAddress(liteDataAddress)
	require.NoError(t, err)
	var account *LiteDataAccount
	require.NoError(t, batch.Account(liteDataAddress).GetStateAs(&account))
	require.Equal(t, liteDataAddress.String(), account.Url.String())
	require.Equal(t, append(partialChainId, account.Tail...), chainId)

	firstEntryHash, err := ComputeFactomEntryHashForAccount(chainId, firstEntry.GetData())
	require.NoError(t, err)

	// Verify the entry hash in the transaction result
	require.IsType(t, (*WriteDataResult)(nil), status.Result)
	txResult := status.Result.(*WriteDataResult)
	require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(txResult.EntryHash[:]), "Transaction result entry hash does not match")

	// Verify the entry hash returned by Entry
	entry, err := indexing.Data(batch, liteDataAddress).GetLatestEntry()
	require.NoError(t, err)
	hashFromEntry, err := ComputeFactomEntryHashForAccount(chainId, entry.GetData())
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(hashFromEntry), "Chain Entry.Hash does not match")
	//sample verification for calculating the entryHash from lite data entry
	entryHash, err := indexing.Data(batch, liteDataAddress).Entry(0)
	require.NoError(t, err)
	txnHash, err := indexing.Data(batch, liteDataAddress).Transaction(entryHash)
	require.NoError(t, err)
	ent, err := indexing.GetDataEntry(batch, txnHash)
	require.NoError(t, err)
	id := ComputeLiteDataAccountId(ent)
	newh, err := ComputeFactomEntryHashForAccount(id, ent.GetData())
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(entryHash), "Chain GetHashes does not match")
	require.Equal(t, hex.EncodeToString(firstEntryHash), hex.EncodeToString(newh), "Chain GetHashes does not match")
}

func TestCreateSubIdentityWithLite(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	liteKey := acctesting.GenerateKey(t.Name(), "Lite")
	liteUrl := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := protocol.AccountUrl("alice")
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
			WithBody(&CreateIdentity{
				Url:        alice.JoinPath("sub"),
				KeyHash:    keyHash[:],
				KeyBookUrl: alice.JoinPath("sub", "book"),
			}).
			Initiate(SignatureTypeLegacyED25519, liteKey).
			Build(),
	)
	var err2 *errors.Error
	require.Error(t, err)
	require.ErrorAs(t, err, &err2)
	require.Equal(t, errors.StatusBadRequest, err2.Code)
}

func TestCreateIdentityWithRemoteLite(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	liteKey := acctesting.GenerateKey(t.Name(), "Lite")
	liteUrl := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), "Alice")
	keyHash := sha256.Sum256(aliceKey[32:])
	sim.CreateAccount(&LiteIdentity{Url: liteUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: liteUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9)})

	_, txn := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice).
			WithTimestampVar(&timestamp).
			WithSigner(liteUrl, 1).
			WithBody(&CreateIdentity{
				Url:        alice,
				KeyHash:    keyHash[:],
				KeyBookUrl: alice.JoinPath("book"),
			}).
			Initiate(SignatureTypeLegacyED25519, liteKey).
			Build(),
	)...)

	// There should not be a synthetic transaction
	require.Len(t, txn, 1)

	// Verify the account is created
	_ = sim.PartitionFor(alice).Database.View(func(batch *database.Batch) error {
		var identity *ADI
		require.NoError(t, batch.Account(alice).GetStateAs(&identity))
		return nil
	})
}

func TestAddCreditsToNewLiteIdentity(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey("alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey("bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob).RootIdentity()
	sim.CreateAccount(&LiteIdentity{Url: aliceUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: aliceUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e12)})

	// Execute
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(aliceUrl).
			WithSigner(aliceUrl.RootIdentity(), 1).
			WithTimestampVar(&timestamp).
			WithBody(&AddCredits{
				Recipient: bobUrl,
				Amount:    *big.NewInt(AcmePrecision * 1e3),
				Oracle:    InitialAcmeOracleValue,
			}).
			Initiate(SignatureTypeED25519, alice).
			Build(),
	)...)

	// Verify
	_ = sim.PartitionFor(bobUrl).Database.View(func(batch *database.Batch) error {
		var account *LiteIdentity
		require.NoError(t, batch.Account(bobUrl).GetStateAs(&account))
		require.Equal(t,
			FormatAmount(1e3*InitialAcmeOracleValue, CreditPrecisionPower),
			FormatAmount(account.CreditBalance, CreditPrecisionPower))
		return nil
	})
}

func TestSubAdi(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	lite := acctesting.GenerateKey(t.Name(), "Lite")
	liteUrl := acctesting.AcmeLiteAddressStdPriv(lite)
	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), alice)
	sim.CreateAccount(&LiteIdentity{Url: liteUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: liteUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9)})
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })

	// Execute
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice).
			WithTimestampVar(&timestamp).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithBody(&CreateIdentity{
				Url: alice.JoinPath("sub"),
			}).
			Initiate(SignatureTypeLegacyED25519, aliceKey).
			Build(),
	)...)

	// Verify
	_ = sim.PartitionFor(alice).Database.View(func(batch *database.Batch) error {
		var identity *ADI
		require.NoError(t, batch.Account(alice.JoinPath("sub")).GetStateAs(&identity))
		require.Len(t, identity.Authorities, 1)
		require.Equal(t, "alice.acme/book", identity.Authorities[0].Url.ShortString())
		return nil
	})
}
