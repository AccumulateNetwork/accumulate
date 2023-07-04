// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestDoubleHashEntries(t *testing.T) {
	var timestamp uint64
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	g := new(core.GlobalValues)
	g.ExecutorVersion = ExecutorVersionV1SignatureAnchoring
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("data")})

	// V1SignatureAnchoring allows AccumulateDataEntries
	st := sim.SubmitTxn(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData(&AccumulateDataEntry{Data: [][]byte{[]byte("foo")}}).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	require.NoError(t, st.AsError())
	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// V1SignatureAnchoring does not allow DoubleHashDataEntries
	st = sim.SubmitTxn(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData(&DoubleHashDataEntry{Data: [][]byte{[]byte("foo")}}).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	require.EqualError(t, st.AsError(), "doubleHash data entries are not accepted")

	// Update the version
	st = sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV1DoubleHashEntries).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(&timestamp).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Give it a few blocks for the anchor to propagate
	sim.StepN(10)

	// Verify version is set
	require.Equal(t, ExecutorVersionV1DoubleHashEntries, GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV1DoubleHashEntries, GetAccount[*SystemLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV1DoubleHashEntries, GetAccount[*SystemLedger](t, sim.Database("BVN1"), PartitionUrl("BVN1").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV1DoubleHashEntries, GetAccount[*SystemLedger](t, sim.Database("BVN2"), PartitionUrl("BVN2").JoinPath(Ledger)).ExecutorVersion)

	// V1DoubleHashEntries does not allow AccumulateDataEntries
	st = sim.SubmitTxn(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData(&AccumulateDataEntry{Data: [][]byte{[]byte("foo")}}).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	require.EqualError(t, st.AsError(), "accumulate data entries are not accepted")

	// V1DoubleHashEntries allows DoubleHashDataEntries
	st = sim.SubmitTxn(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData(&DoubleHashDataEntry{Data: [][]byte{[]byte("foo")}}).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	require.NoError(t, st.AsError())
	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Update the version
	st = sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV1Halt).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(&timestamp).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Give it a few blocks for the anchor to propagate
	sim.StepN(10)

	// Verify version is set
	require.Equal(t, ExecutorVersionV1Halt, GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV1Halt, GetAccount[*SystemLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV1Halt, GetAccount[*SystemLedger](t, sim.Database("BVN1"), PartitionUrl("BVN1").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV1Halt, GetAccount[*SystemLedger](t, sim.Database("BVN2"), PartitionUrl("BVN2").JoinPath(Ledger)).ExecutorVersion)

	// Give it a while for synthetic transactions to settle
	sim.StepN(50)

	// Update the version
	st = sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV2).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(&timestamp).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Give it a few blocks for the anchor to propagate
	sim.StepN(10)

	// Verify version is set
	require.Equal(t, ExecutorVersionV2, GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV2, GetAccount[*SystemLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV2, GetAccount[*SystemLedger](t, sim.Database("BVN1"), PartitionUrl("BVN1").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV2, GetAccount[*SystemLedger](t, sim.Database("BVN2"), PartitionUrl("BVN2").JoinPath(Ledger)).ExecutorVersion)

	// V2 does not allow AccumulateDataEntries
	st = sim.SubmitTxn(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData(&AccumulateDataEntry{Data: [][]byte{[]byte("foo")}}).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	require.EqualError(t, st.AsError(), "accumulate data entries are not accepted")

	// V2 allows DoubleHashDataEntries
	st = sim.SubmitTxn(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData(&DoubleHashDataEntry{Data: [][]byte{[]byte("foo")}}).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	require.NoError(t, st.AsError())
	sim.StepUntil(
		Txn(st.TxID).Succeeds())
}

func Test64ByteBody(t *testing.T) {
	var timestamp uint64
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Construct a transaction of 64 bytes
	b, err := new(BurnTokens).MarshalBinary()
	require.NoError(t, err)
	b = append(b, 2)
	c := make([]byte, 64-len(b))
	c[0] = byte(len(c) - 1)
	for i := range c[1:] {
		c[i+1] = byte(i) + 1
	}
	b = append(b, c...)

	body := new(BurnTokens)
	require.NoError(t, body.UnmarshalBinary(b))
	b, err = body.MarshalBinary()
	require.NoError(t, err)
	require.Len(t, b, 64) // Sanity check

	// Marshal and unmarshal
	txn := new(Transaction)
	txn.Header.Principal = alice.JoinPath("tokens")
	txn.Body = body
	b, err = txn.MarshalBinary()
	require.NoError(t, err)
	txn = new(Transaction)
	require.NoError(t, txn.UnmarshalBinary(b))
	require.True(t, txn.BodyIs64Bytes())
	txn = txn.Copy() // Reset hash

	// Sign
	sig := new(ED25519Signature)
	sig.Signer = alice.JoinPath("book", "1")
	sig.SignerVersion = 1
	sig.Timestamp = timestamp
	sig.PublicKey = aliceKey[32:]
	txn.Header.Initiator = *(*[32]byte)(sig.Metadata().Hash())

	sig.TransactionHash = txn.ID().Hash()
	SignED25519(sig, aliceKey, txn.Header.Initiator[:], sig.TransactionHash[:])

	// Initialize
	g := new(core.GlobalValues)
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens")})

	env := &messaging.Envelope{Transaction: []*Transaction{txn}, Signatures: []Signature{sig}}
	require.Equal(t, env.Transaction[0].ID().Hash(), env.Signatures[0].GetTransactionHash())
	st := sim.SubmitTxn(env)
	require.EqualError(t, st.AsError(), "cannot process transaction: body is 64 bytes long")
}

func Test65ByteBody(t *testing.T) {
	var timestamp uint64
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Construct a transaction of 64 bytes
	b, err := new(BurnTokens).MarshalBinary()
	require.NoError(t, err)
	b = append(b, 2)
	c := make([]byte, 64-len(b))
	c[0] = byte(len(c) - 1)
	for i := range c[1:] {
		c[i+1] = byte(i) + 1
	}
	b = append(b, c...)

	// Tack a zero on the end
	b = append(b, 0)

	body := new(BurnTokens)
	require.NoError(t, body.UnmarshalBinary(b))
	b, err = body.MarshalBinary()
	require.NoError(t, err)
	require.Len(t, b, 65) // Sanity check

	// Marshal and unmarshal
	txn := new(Transaction)
	txn.Header.Principal = alice.JoinPath("tokens")
	txn.Body = body
	b, err = txn.MarshalBinary()
	require.NoError(t, err)
	txn = new(Transaction)
	require.NoError(t, txn.UnmarshalBinary(b))

	// Verify the body is still 65 bytes
	b, err = txn.Body.MarshalBinary()
	require.NoError(t, err)
	require.Len(t, b, 65) // Sanity check
	require.False(t, txn.BodyIs64Bytes())
	txn = txn.Copy() // Reset the cached hash

	// Initialize
	g := new(core.GlobalValues)
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), Balance: body.Amount, TokenUrl: AcmeUrl()})

	st := sim.SubmitTxn(MustBuild(t,
		build.SignatureForTransaction(txn).
			Url(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))
	require.NoError(t, st.AsError())
	sim.StepUntil(
		Txn(st.TxID).Succeeds())
}

func Test64ByteHeader(t *testing.T) {
	var timestamp uint64

	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	aliceX := alice.JoinPath(strings.Repeat("x", 28-len(alice.String())))

	env := MustBuild(t, build.Transaction().For(aliceX).
		BurnTokens(1, 0).
		SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))
	b, err := env.Transaction[0].Header.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, 64, len(b), "Header should be 64 bytes")

	// Initialize
	g := new(core.GlobalValues)
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: aliceX, Balance: *big.NewInt(1), TokenUrl: AcmeUrl()})

	// Execute
	st := sim.SubmitTxnSuccessfully(env)
	sim.StepUntil(
		Txn(st.TxID).Succeeds())
}
