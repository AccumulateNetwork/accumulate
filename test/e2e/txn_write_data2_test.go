package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
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
	st := sim.Submit(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData(&AccumulateDataEntry{Data: [][]byte{[]byte("foo")}}).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	require.NoError(t, st.AsError())
	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// V1SignatureAnchoring does not allow DoubleHashDataEntries
	st = sim.Submit(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData(&DoubleHashDataEntry{Data: [][]byte{[]byte("foo")}}).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	require.EqualError(t, st.AsError(), "doubleHash data entries are not accepted")

	// Update the version
	st = sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV1DoubleHashEntries).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))))

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
	st = sim.Submit(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData(&AccumulateDataEntry{Data: [][]byte{[]byte("foo")}}).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	require.EqualError(t, st.AsError(), "accumulate data entries are not accepted")

	// V1DoubleHashEntries allows DoubleHashDataEntries
	st = sim.Submit(MustBuild(t,
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

	_, is64 := txn.GetHash2()
	require.True(t, is64)

	// Initialize
	g := new(core.GlobalValues)
	g.ExecutorVersion = ExecutorVersionV1DoubleHashEntries
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens")})

	st := sim.Submit(MustBuild(t,
		build.SignatureForTransaction(txn).
			Url(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))
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

	_, is64 := txn.GetHash2()
	require.False(t, is64)
	txn = txn.Copy() // Reset the cached hash

	// Initialize
	g := new(core.GlobalValues)
	g.ExecutorVersion = ExecutorVersionV1DoubleHashEntries
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), Balance: body.Amount, TokenUrl: AcmeUrl()})

	st := sim.Submit(MustBuild(t,
		build.SignatureForTransaction(txn).
			Url(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))
	require.NoError(t, st.AsError())
	sim.StepUntil(
		Txn(st.TxID).Succeeds())
}
