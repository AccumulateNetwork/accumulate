// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	dut "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestQuerier(t *testing.T) {
	suite.Run(t, new(QuerierTestSuite))
}

type QuerierTestSuite struct {
	suite.Suite
	sim    *simulator.Simulator
	faucet *url.URL
	parts  []*protocol.PartitionInfo

	alice, bob, charlie          *url.URL
	aliceKey, bobKey, charlieKey ed25519.PrivateKey

	createAlice *TransactionStatus
	writeData   *TransactionStatus
	createMulti *TransactionStatus
}

func (s *QuerierTestSuite) QuerierFor(u *url.URL) api.Querier2 {
	part, err := s.sim.Router().RouteAccount(u)
	s.Require().NoError(err)
	q := dut.NewQuerier(dut.QuerierParams{
		Logger:    acctesting.NewTestLogger(s.T()),
		Database:  s.sim.Database(part),
		Partition: part,
	})
	return api.Querier2{Querier: q}
}

func (s *QuerierTestSuite) SetupSuite() {
	var timestamp uint64

	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "* * * * *"
	g.ExecutorVersion = ExecutorVersionV2

	var err error
	s.sim, err = simulator.New(
		simulator.WithLogger(acctesting.NewTestLogger(s.T())),
		simulator.SimpleNetwork(s.T().Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)
	s.Require().NoError(err)
	sim := NewSimWith(s.T(), s.sim)

	faucetKey := acctesting.GenerateKey("faucet")
	s.faucet = acctesting.AcmeLiteAddressStdPriv(faucetKey)

	s.alice = AccountUrl("alice")
	s.bob = AccountUrl("bob")
	s.charlie = AccountUrl("charlie")
	s.aliceKey = acctesting.GenerateKey(s.alice)
	s.bobKey = acctesting.GenerateKey(s.bob)
	s.charlieKey = acctesting.GenerateKey(s.charlie)

	s.parts = sim.Partitions()
	for i, p := range s.parts {
		if p.Type == PartitionTypeDirectory {
			sortutil.RemoveAt(&s.parts, i)
			break
		}
	}
	sim.SetRoute(s.alice, s.parts[0].ID)
	sim.SetRoute(s.faucet, s.parts[1].ID)
	sim.SetRoute(s.bob, s.parts[1].ID)
	sim.SetRoute(s.charlie, s.parts[2].ID)

	MakeLiteTokenAccount(s.T(), sim.DatabaseFor(s.faucet), faucetKey[32:], AcmeUrl())
	CreditTokens(s.T(), sim.DatabaseFor(s.faucet), s.faucet, big.NewInt(1000*AcmePrecision))

	// Get credits
	st := sim.SubmitTxnSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.faucet).
			AddCredits().To(s.faucet).WithOracle(InitialAcmeOracle).Spend(10).
			SignWith(s.faucet).Version(1).Timestamp(&timestamp).PrivateKey(faucetKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Create Alice
	st = sim.SubmitTxnSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.faucet).
			CreateIdentity(s.alice).WithKey(s.aliceKey, SignatureTypeED25519).WithKeyBook(s.alice, "book").
			SignWith(s.faucet).Version(1).Timestamp(&timestamp).PrivateKey(faucetKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
	s.createAlice = st

	st = sim.SubmitTxnSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.faucet).
			AddCredits().To(s.alice, "book", "1").WithOracle(InitialAcmeOracle).Spend(10).
			SignWith(s.faucet).Version(1).Timestamp(&timestamp).PrivateKey(faucetKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Create alice/data
	st = sim.SubmitTxnSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.alice).
			CreateDataAccount(s.alice, "data").
			SignWith(s.alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(s.aliceKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds())
	s.writeData = st

	// Write to alice/data
	st = sim.SubmitTxnSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.alice, "data").
			WriteData().DoubleHash([]byte("foo")).
			SignWith(s.alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(s.aliceKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Create Bob and Charlie
	st = sim.SubmitTxnSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.faucet).
			CreateIdentity(s.bob).WithKey(s.bobKey, SignatureTypeED25519).WithKeyBook(s.bob, "book").
			SignWith(s.faucet).Version(1).Timestamp(&timestamp).PrivateKey(faucetKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	st = sim.SubmitTxnSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.faucet).
			CreateIdentity(s.charlie).WithKey(s.charlieKey, SignatureTypeED25519).WithKeyBook(s.charlie, "book").
			SignWith(s.faucet).Version(1).Timestamp(&timestamp).PrivateKey(faucetKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	st = sim.SubmitTxnSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.faucet).
			AddCredits().To(s.bob, "book", "1").WithOracle(InitialAcmeOracle).Spend(10).
			SignWith(s.faucet).Version(1).Timestamp(&timestamp).PrivateKey(faucetKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	st = sim.SubmitTxnSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.faucet).
			AddCredits().To(s.charlie, "book", "1").WithOracle(InitialAcmeOracle).Spend(10).
			SignWith(s.faucet).Version(1).Timestamp(&timestamp).PrivateKey(faucetKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Create alice/multi
	env := MustBuild(s.T(),
		build.Transaction().For(s.alice).
			CreateDataAccount(s.alice, "mutli").
			WithAuthority(s.bob, "book").
			WithAuthority(s.charlie, "book").
			SignWith(s.alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(s.aliceKey))
	st = sim.SubmitTxnSuccessfully(env)

	sim.BuildAndSubmitSuccessfully(
		build.SignatureForTransaction(env.Transaction[0]).
			Url(s.bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(s.bobKey))

	sim.BuildAndSubmitSuccessfully(
		build.SignatureForTransaction(env.Transaction[0]).
			Url(s.charlie, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(s.charlieKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())
	s.createMulti = st

	// Ensure a major block
	sim.StepN(int(time.Minute / time.Second))
}

func uintp(v uint64) *uint64 { return &v }
func boolp(v bool) *bool     { return &v }

func (s *QuerierTestSuite) TestQueryTransaction() {
	r, err := s.QuerierFor(s.faucet).QueryTransaction(context.Background(), s.createAlice.TxID, nil)
	s.Require().NoError(err)
	_ = s.NotNil(r.Message.Transaction) &&
		s.IsType((*CreateIdentity)(nil), r.Message.Transaction.Body)
	_ = s.NotNil(r.Status) &&
		s.Equal(errors.Delivered, r.Status)
	_ = s.NotNil(r.Produced) &&
		s.Len(r.Produced.Records, 2)
	_ = s.NotNil(r.Signatures) &&
		s.Len(r.Signatures.Records, 2)
}

func (s *QuerierTestSuite) TestQueryAccount() {
	r, err := s.QuerierFor(s.alice).QueryAccount(context.Background(), s.alice, nil)
	s.Require().NoError(err)
	_ = s.NotNil(r.Account) &&
		s.IsType((*ADI)(nil), r.Account)
	_ = s.NotNil(r.Directory) &&
		s.Len(r.Directory.Records, 3) &&
		s.True(r.Directory.Records[0].Value.Equal(s.alice.JoinPath("book")))
}

func (s *QuerierTestSuite) TestQueryAccountChains() {
	r, err := s.QuerierFor(s.alice).QueryAccountChains(context.Background(), s.alice, nil)
	s.Require().NoError(err)
	_ = s.Len(r.Records, 4) &&
		s.Equal("main", r.Records[0].Name) &&
		s.Equal(merkle.ChainTypeTransaction, r.Records[0].Type) &&
		s.Equal(3, int(r.Records[0].Count))
}

func (s *QuerierTestSuite) TestQueryTransactionChains() {
	r, err := s.QuerierFor(s.alice).QueryChainEntries(context.Background(),
		protocol.PartitionUrl(s.parts[0].ID).JoinPath(protocol.Ledger),
		&api.ChainQuery{Name: "root", Range: &api.RangeOptions{FromEnd: true, Count: api.Ptr[uint64](1)}})
	s.Require().NoError(err)
	s.Require().Len(r.Records, 1)

	r, err = s.QuerierFor(s.alice).QueryTransactionChains(context.Background(), s.writeData.TxID, &api.ChainQuery{IncludeReceipt: &api.ReceiptOptions{ForHeight: r.Records[0].Index}})
	s.Require().NoError(err)
	s.Require().Len(r.Records, 2)

	r1 := r.Records[0]
	_ = s.Equal(s.alice.ShortString(), r1.Account.ShortString()) &&
		s.Equal("main", r1.Name) &&
		s.Equal(merkle.ChainTypeTransaction, r1.Type) &&
		s.Equal(1, int(r1.Index)) &&
		s.Equal(s.writeData.TxID.Hash(), r1.Entry) &&
		s.NotNil(r1.Receipt)

	r2 := r.Records[1]
	_ = s.Equal(s.alice.JoinPath("data").ShortString(), r2.Account.ShortString()) &&
		s.Equal("main", r2.Name) &&
		s.Equal(merkle.ChainTypeTransaction, r2.Type) &&
		s.Equal(0, int(r2.Index)) &&
		s.Equal(s.writeData.TxID.Hash(), r1.Entry) &&
		s.NotNil(r2.Receipt)
}

func (s *QuerierTestSuite) TestQueryChain() {
	r, err := s.QuerierFor(s.alice).QueryChain(context.Background(), s.alice, &api.ChainQuery{Name: "main"})
	s.Require().NoError(err)
	_ = s.Equal("main", r.Name) &&
		s.Equal(merkle.ChainTypeTransaction, r.Type) &&
		s.Equal(3, int(r.Count))
}

func (s *QuerierTestSuite) TestQueryChainEntry() {
	r, err := s.QuerierFor(s.faucet).QueryTransaction(context.Background(), s.createAlice.TxID, nil)
	s.Require().NoError(err)
	txn := r.Produced.Records[0].Value
	hash := txn.Hash()

	s.Run("ByIndex", func() {
		r, err := s.QuerierFor(s.alice).QueryChainEntry(context.Background(), s.alice, &api.ChainQuery{Name: "main", Index: uintp(0)})
		s.Require().NoError(err)
		s.Equal(txn.Hash(), r.Entry)
		s.IsType((*api.MessageRecord[messaging.Message])(nil), r.Value)
	})

	s.Run("ByValue", func() {
		r, err := s.QuerierFor(s.alice).QueryChainEntry(context.Background(), s.alice, &api.ChainQuery{Name: "main", Entry: hash[:]})
		s.Require().NoError(err)
		s.Equal(0, int(r.Index))
		s.IsType((*api.MessageRecord[messaging.Message])(nil), r.Value)
	})
}

func (s *QuerierTestSuite) TestQueryChainEntries() {
	r, err := s.QuerierFor(s.alice).QueryMainChainEntries(context.Background(), s.alice, &api.ChainQuery{Name: "main", Range: &api.RangeOptions{}})
	s.Require().NoError(err)
	s.Require().Len(r.Records, 3)
}

func (s *QuerierTestSuite) TestQueryDataEntry() {
	entry := &DoubleHashDataEntry{Data: [][]byte{[]byte("foo")}}

	s.Run("Latest", func() {
		r, err := s.QuerierFor(s.alice).QueryDataEntry(context.Background(), s.alice.JoinPath("data"), nil)
		s.Require().NoError(err)
		s.Equal(0, int(r.Index))
		s.Equal(entry.Hash(), r.Entry[:])
	})

	s.Run("ByIndex", func() {
		r, err := s.QuerierFor(s.alice).QueryDataEntry(context.Background(), s.alice.JoinPath("data"), &api.DataQuery{Index: uintp(0)})
		s.Require().NoError(err)
		s.Equal(entry.Hash(), r.Entry[:])
	})

	s.Run("ByValue", func() {
		r, err := s.QuerierFor(s.alice).QueryDataEntry(context.Background(), s.alice.JoinPath("data"), &api.DataQuery{Entry: entry.Hash()})
		s.Require().NoError(err)
		s.Equal(0, int(r.Index))
	})
}

func (s *QuerierTestSuite) TestQueryDataEntries() {
	r, err := s.QuerierFor(s.alice).QueryDataEntries(context.Background(), s.alice.JoinPath("data"), &api.DataQuery{Range: &api.RangeOptions{}})
	s.Require().NoError(err)
	s.Require().Len(r.Records, 1)
}

func (s *QuerierTestSuite) TestQueryDirectory() {
	r, err := s.QuerierFor(s.alice).QueryDirectory(context.Background(), s.alice, nil)
	s.Require().NoError(err)
	s.Require().Len(r.Records, 3)
}

func (s *QuerierTestSuite) TestQueryPending() {
	r, err := s.QuerierFor(s.alice).QueryPending(context.Background(), s.alice, nil)
	s.Require().NoError(err)
	s.Require().Len(r.Records, 0)
}

func (s *QuerierTestSuite) TestQueryMinorBlock() {
	r, err := s.QuerierFor(s.alice).QueryMinorBlock(context.Background(), s.alice, &api.BlockQuery{Minor: uintp(2)})
	s.Require().NoError(err)
	s.Require().NotEmpty(r.Entries.Records)
}

func (s *QuerierTestSuite) TestQueryDirectoryMinorBlock() {
	r, err := s.QuerierFor(DnUrl()).QueryMinorBlock(context.Background(), DnUrl(), &api.BlockQuery{Minor: uintp(4)})
	s.Require().NoError(err)
	s.Require().NotNil(r.Anchored)
	s.Require().NotEmpty(r.Anchored.Records)
}

func (s *QuerierTestSuite) TestQueryMinorBlocks() {
	r, err := s.QuerierFor(s.alice).QueryMinorBlocks(context.Background(), s.alice, &api.BlockQuery{MinorRange: &api.RangeOptions{}})
	s.Require().NoError(err)
	s.Require().Len(r.Records, 50)
}

func (s *QuerierTestSuite) TestQueryMajorBlock() {
	s.Run("Simple", func() {
		r, err := s.QuerierFor(s.alice).QueryMajorBlock(context.Background(), s.alice, &api.BlockQuery{Major: uintp(1)})
		s.Require().NoError(err)
		s.Require().Len(r.MinorBlocks.Records, 50)
	})
	s.Run("WithMinorRange", func() {
		r, err := s.QuerierFor(s.alice).QueryMajorBlock(context.Background(), s.alice, &api.BlockQuery{Major: uintp(1), MinorRange: &api.RangeOptions{Count: uintp(10)}})
		s.Require().NoError(err)
		s.Require().Len(r.MinorBlocks.Records, 10)
	})
}

func (s *QuerierTestSuite) TestQueryMajorBlocks() {
	r, err := s.QuerierFor(s.alice).QueryMajorBlocks(context.Background(), s.alice, &api.BlockQuery{MajorRange: &api.RangeOptions{}})
	s.Require().NoError(err)
	s.Require().NotEmpty(r.Records)
}

func (s *QuerierTestSuite) TestSearchForAnchor() {
	u := PartitionUrl("BVN1").JoinPath(AnchorPool)
	chr, err := s.QuerierFor(u).QueryMainChainEntries(context.Background(), u, &api.ChainQuery{Name: "anchor-sequence", Range: &api.RangeOptions{FromEnd: true, Count: uintp(1), Start: 0, Expand: boolp(true)}})
	s.Require().NoError(err)
	anchor := chr.Records[0].Value.Message.Transaction.Body.(*BlockValidatorAnchor).RootChainAnchor

	r, err := s.QuerierFor(DnUrl()).SearchForAnchor(context.Background(), DnUrl().JoinPath(AnchorPool), &api.AnchorSearchQuery{Anchor: anchor[:]})
	s.Require().NoError(err)
	s.NotEmpty(r.Records)
}

func (s *QuerierTestSuite) TestQueryIncludesAnchorSignatures() {
	r, err := s.QuerierFor(DnUrl()).QueryMainChainEntry(context.Background(), DnUrl().JoinPath(AnchorPool), &api.ChainQuery{Name: "main", Index: uintp(1)})
	s.Require().NoError(err)

	var hasAnchor bool
	for _, set := range r.Value.Signatures.Records {
		for _, sig := range set.Signatures.Records {
			if sig.Message.Type() == messaging.MessageTypeBlockAnchor {
				hasAnchor = true
			}
		}
	}
	s.Require().True(hasAnchor, "Expected anchor transaction to include anchor signatures")
}

func (s *QuerierTestSuite) TestSearchForPublicKey() {
	r, err := s.QuerierFor(s.alice).SearchForPublicKey(context.Background(), s.alice, &api.PublicKeySearchQuery{PublicKey: s.aliceKey[32:], Type: SignatureTypeED25519})
	s.Require().NoError(err)
	s.Require().Len(r.Records, 1)
}

func (s *QuerierTestSuite) TestSearchForPublicKeyHash() {
	hash := sha256.Sum256(s.aliceKey[32:])
	r, err := s.QuerierFor(s.alice).SearchForPublicKeyHash(context.Background(), s.alice, &api.PublicKeyHashSearchQuery{PublicKeyHash: hash[:]})
	s.Require().NoError(err)
	s.Require().Len(r.Records, 1)
}

func (s *QuerierTestSuite) TestSearchForTransactionHash() {
	r, err := s.QuerierFor(s.faucet).SearchForMessage(context.Background(), s.createAlice.TxID.Hash())
	s.Require().NoError(err)
	s.Require().Len(r.Records, 1)
}

func (s *QuerierTestSuite) TestCollateTransaction() {
	c := &api.Collator{Querier: s.sim.Services(), Network: s.sim.Services()}
	q := api.Querier2{Querier: c}
	r, err := q.QueryTransaction(context.Background(), UnknownUrl().WithTxID(s.createMulti.TxID.Hash()), nil)
	s.Require().NoError(err)

	msgs := map[[32]byte][]messaging.Message{}
	for _, r := range r.Signatures.Records {
		var m []messaging.Message
		for _, r := range r.Signatures.Records {
			m = append(m, r.Message)
		}
		msgs[r.Account.GetUrl().AccountID32()] = m
	}

	s.Require().NotEmpty(msgs[s.alice.JoinPath("book", "1").AccountID32()], "Expected messages from Alice's page")
	s.Require().NotEmpty(msgs[s.bob.JoinPath("book", "1").AccountID32()], "Expected messages from Bob's page")
	s.Require().NotEmpty(msgs[s.charlie.JoinPath("book", "1").AccountID32()], "Expected messages from Charlie's page")
}
