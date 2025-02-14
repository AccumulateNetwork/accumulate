// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"testing"

	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	oldsim "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() { acctesting.EnableDebugFeatures() }

var delivered = (*TransactionStatus).Delivered
var pending = (*TransactionStatus).Pending

func updateAccount[T Account](sim *simulator.Simulator, accountUrl *url.URL, fn func(account T)) {
	sim.UpdateAccount(accountUrl, func(account Account) {
		var typed T
		err := encoding.SetPtr(account, &typed)
		require.NoError(sim.TB, err)
		fn(typed)
	})
}

func updateAccountOld[T Account](sim *oldsim.Simulator, accountUrl *url.URL, fn func(account T)) {
	sim.UpdateAccount(accountUrl, func(account Account) {
		var typed T
		err := encoding.SetPtr(account, &typed)
		require.NoError(sim, err)
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
	exch.AddRecipient(AccountUrl("foo"), big.NewInt(int64(1000)))
	env :=
		MustBuild(t, build.Transaction().
			For(aliceUrl).
			Body(exch).
			SignWith(aliceUrl.RootIdentity()).Version(1).Timestamp(&timestamp).PrivateKey(alice).Type(SignatureTypeLegacyED25519))

	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	// The balance should be unchanged
	batch = sim.PartitionFor(aliceUrl).Database.Begin(false)
	defer batch.Discard()
	var account *LiteTokenAccount
	require.NoError(t, batch.Account(aliceUrl).Main().GetAs(&account))
	assert.Equal(t, int64(AcmeFaucetAmount*AcmePrecision), account.Balance.Int64())

	// The synthetic transaction should fail
	synth, err := batch.Transaction(env.Transaction[0].GetHash()).Produced().Get()
	require.NoError(t, err)
	batch = sim.PartitionFor(AccountUrl("foo")).Database.Begin(false)
	defer batch.Discard()
	h := synth[0].Hash()
	status, err := batch.Transaction(h[:]).Status().Get()
	require.NoError(t, err)
	assert.Equal(t, errors.NotFound, status.Code)
}

func TestDoesChargeFee(t *testing.T) {
	const initialBalance = 1000
	var timestamp uint64
	aliceKey := acctesting.GenerateKey("alice")
	bobKey := acctesting.GenerateKey("bob")
	alice := acctesting.AcmeLiteAddressStdPriv(aliceKey)
	bob := acctesting.AcmeLiteAddressStdPriv(bobKey)

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()
	sim.CreateAccount(&LiteIdentity{Url: alice.RootIdentity(), CreditBalance: initialBalance})
	sim.CreateAccount(&LiteTokenAccount{Url: alice, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e12)})

	// Send tokens
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&SendTokens{To: []*TokenRecipient{{
				Url:    bob,
				Amount: *big.NewInt(1),
			}}}).
			SignWith(alice).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)),
	)...)

	lid := simulator.GetAccount[*LiteIdentity](sim, alice.RootIdentity())
	require.Equal(t, int(initialBalance-FeeTransferTokens), int(lid.CreditBalance))
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
		require.NoError(t, batch.Account(aliceUrl.RootIdentity()).Main().GetAs(&account))
		creditsBefore = account.CreditBalance
		return nil
	})

	exch := new(SendTokens)
	exch.AddRecipient(AccountUrl("foo"), big.NewInt(int64(1000)))
	exch.AddRecipient(bobUrl, big.NewInt(int64(1000)))
	env :=
		MustBuild(t, build.Transaction().
			For(aliceUrl).
			Body(exch).
			SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice).Type(SignatureTypeLegacyED25519))

	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	s := sim.PartitionFor(aliceUrl).Globals().Globals.FeeSchedule
	fee, err := s.ComputeTransactionFee(env.Transaction[0])
	require.NoError(t, err)
	refund, err := s.ComputeSyntheticRefund(env.Transaction[0], len(exch.To))
	require.NoError(t, err)

	lid := simulator.GetAccount[*LiteIdentity](sim, aliceUrl.RootIdentity())
	require.Equal(t, int(creditsBefore-(fee-refund).AsUInt64()), int(lid.CreditBalance))
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

	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), alice)
	keyHash := sha256.Sum256(aliceKey[32:])

	_, txn := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&CreateIdentity{
				Url:        alice,
				KeyHash:    keyHash[:],
				KeyBookUrl: alice.JoinPath("book"),
			}).
			SignWith(liteUrl.RootIdentity()).Version(1).Timestamp(&timestamp).PrivateKey(lite).Type(SignatureTypeLegacyED25519)),
	)...)

	// There should not be a synthetic transaction
	require.Len(t, txn, 1)

	// Verify the account is created
	_ = sim.PartitionFor(alice).Database.View(func(batch *database.Batch) error {
		var identity *ADI
		require.NoError(t, batch.Account(alice).Main().GetAs(&identity))
		return nil
	})
}

func TestWriteToLiteDataAccount(t *testing.T) {
	// Setup
	alice := acctesting.GenerateKey(t.Name())
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(tmed25519.PrivKey(alice))
	aliceAdi := AccountUrl("alice")

	firstEntry := DoubleHashDataEntry{}
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
		require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(alice), 1e9, 1e9))
		require.NoError(t, batch.Commit())

		// Write data
		env :=
			MustBuild(t, build.Transaction().
				For(liteDataAddress).
				Body(&WriteData{Entry: &firstEntry}).
				SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice))

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
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(alice), "alice", 1e9))
		require.NoError(t, batch.Commit())

		// Write data
		env :=
			MustBuild(t, build.Transaction().
				For(liteDataAddress).
				Body(&WriteData{Entry: &firstEntry}).
				SignWith(aliceAdi.JoinPath("book0", "1")).Version(1).Timestamp(&timestamp).PrivateKey(alice))

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
	require.NoError(t, batch.Account(liteDataAddress).Main().GetAs(&account))
	require.Equal(t, liteDataAddress.String(), account.Url.String())
	require.Equal(t, partialChainId, chainId)

	// Verify the entry hash in the transaction result
	require.IsType(t, (*WriteDataResult)(nil), status.Result)
	txResult := status.Result.(*WriteDataResult)
	require.Equal(t, hex.EncodeToString(firstEntry.Hash()), hex.EncodeToString(txResult.EntryHash[:]), "Transaction result entry hash does not match")

	// Verify the entry hash returned by Entry
	entry, _, _, err := indexing.Data(batch, liteDataAddress).GetLatestEntry()
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(firstEntry.Hash()), hex.EncodeToString(entry.Hash()), "Chain Entry.Hash does not match")
	//sample verification for calculating the entryHash from lite data entry
	entryHash, err := indexing.Data(batch, liteDataAddress).Entry(0)
	require.NoError(t, err)
	txnHash, err := indexing.Data(batch, liteDataAddress).Transaction(entryHash)
	require.NoError(t, err)
	ent, _, _, err := indexing.GetDataEntry(batch, txnHash)
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(firstEntry.Hash()), hex.EncodeToString(entryHash), "Chain GetHashes does not match")
	require.Equal(t, hex.EncodeToString(firstEntry.Hash()), hex.EncodeToString(ent.Hash()), "Chain GetHashes does not match")
}

func TestCreateSubIdentityWithLite(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	liteKey := acctesting.GenerateKey(t.Name(), "Lite")
	liteUrl := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), "Alice")
	keyHash := sha256.Sum256(aliceKey[32:])
	sim.CreateIdentity(alice, aliceKey[32:])
	sim.CreateAccount(&LiteIdentity{Url: liteUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: liteUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9)})

	_, err := sim.SubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(liteUrl).
			Body(&CreateIdentity{
				Url:        alice.JoinPath("sub"),
				KeyHash:    keyHash[:],
				KeyBookUrl: alice.JoinPath("sub", "book"),
			}).
			SignWith(liteUrl).Version(1).Timestamp(&timestamp).PrivateKey(liteKey).Type(SignatureTypeLegacyED25519)),
	)
	var err2 *errors.Error
	require.Error(t, err)
	require.ErrorAs(t, err, &err2)
	require.Equal(t, errors.BadRequest, err2.Code)
}

func TestCreateIdentityWithRemoteLite(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	liteKey := acctesting.GenerateKey(t.Name(), "Lite")
	liteUrl := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), "Alice")
	keyHash := sha256.Sum256(aliceKey[32:])
	sim.CreateAccount(&LiteIdentity{Url: liteUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: liteUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9)})

	_, txn := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&CreateIdentity{
				Url:        alice,
				KeyHash:    keyHash[:],
				KeyBookUrl: alice.JoinPath("book"),
			}).
			SignWith(liteUrl).Version(1).Timestamp(&timestamp).PrivateKey(liteKey).Type(SignatureTypeLegacyED25519)),
	)...)

	// There should not be a synthetic transaction
	require.Len(t, txn, 1)

	// Verify the account is created
	_ = sim.PartitionFor(alice).Database.View(func(batch *database.Batch) error {
		var identity *ADI
		require.NoError(t, batch.Account(alice).Main().GetAs(&identity))
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
		MustBuild(t, build.Transaction().
			For(aliceUrl).
			Body(&AddCredits{
				Recipient: bobUrl,
				Amount:    *big.NewInt(AcmePrecision * 1e3),
				Oracle:    InitialAcmeOracleValue,
			}).
			SignWith(aliceUrl.RootIdentity()).Version(1).Timestamp(&timestamp).PrivateKey(alice)),
	)...)

	// Verify
	_ = sim.PartitionFor(bobUrl).Database.View(func(batch *database.Batch) error {
		var account *LiteIdentity
		require.NoError(t, batch.Account(bobUrl).Main().GetAs(&account))
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
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), alice)
	sim.CreateAccount(&LiteIdentity{Url: liteUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: liteUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9)})
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })

	// Execute
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&CreateIdentity{
				Url: alice.JoinPath("sub"),
			}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey).Type(SignatureTypeLegacyED25519)),
	)...)

	// Verify
	_ = sim.PartitionFor(alice).Database.View(func(batch *database.Batch) error {
		var identity *ADI
		require.NoError(t, batch.Account(alice.JoinPath("sub")).Main().GetAs(&identity))
		require.Empty(t, identity.Authorities)
		return nil
	})
}
