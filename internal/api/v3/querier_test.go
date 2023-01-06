// Copyright 2023 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
	sim         *simulator.Simulator
	faucet      *url.URL
	alice       *url.URL
	aliceKey    ed25519.PrivateKey
	createAlice *TransactionStatus
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
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "* * * * *"

	var err error
	s.sim, err = simulator.New(
		acctesting.NewTestLogger(s.T()),
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(s.T().Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)
	s.Require().NoError(err)
	sim := NewSimWith(s.T(), s.sim)

	faucetKey := acctesting.GenerateKey("faucet")
	s.faucet = acctesting.AcmeLiteAddressStdPriv(faucetKey)

	s.alice = AccountUrl("alice")
	s.aliceKey = acctesting.GenerateKey(s.alice)

	parts := sim.Partitions()
	for i, p := range parts {
		if p.Type == PartitionTypeDirectory {
			sortutil.RemoveAt(&parts, i)
			break
		}
	}
	sim.SetRoute(s.alice, parts[0].ID)
	sim.SetRoute(s.faucet, parts[1].ID)

	MakeLiteTokenAccount(s.T(), sim.DatabaseFor(s.faucet), faucetKey[32:], AcmeUrl())
	CreditTokens(s.T(), sim.DatabaseFor(s.faucet), s.faucet, big.NewInt(1000*AcmePrecision))

	st := sim.SubmitSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.faucet).
			AddCredits().To(s.faucet).WithOracle(InitialAcmeOracle).Spend(10).
			SignWith(s.faucet).Version(1).Timestamp(1).PrivateKey(faucetKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	st = sim.SubmitSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.faucet).
			CreateIdentity(s.alice).WithKey(s.aliceKey, SignatureTypeED25519).WithKeyBook(s.alice, "book").
			SignWith(s.faucet).Version(1).Timestamp(2).PrivateKey(faucetKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
	s.createAlice = st

	st = sim.SubmitSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.faucet).
			AddCredits().To(s.alice, "book", "1").WithOracle(InitialAcmeOracle).Spend(10).
			SignWith(s.faucet).Version(1).Timestamp(3).PrivateKey(faucetKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	st = sim.SubmitSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.alice).
			CreateDataAccount(s.alice, "data").
			SignWith(s.alice, "book", "1").Version(1).Timestamp(1).PrivateKey(s.aliceKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	st = sim.SubmitSuccessfully(MustBuild(s.T(),
		build.Transaction().For(s.alice, "data").
			WriteData([]byte("foo")).
			SignWith(s.alice, "book", "1").Version(1).Timestamp(2).PrivateKey(s.aliceKey)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Ensure a major block
	sim.StepN(int(time.Minute / time.Second))
}

func uintp(v uint64) *uint64 { return &v }

func (s *QuerierTestSuite) TestQueryTransaction() {
	r, err := s.QuerierFor(s.faucet).QueryTransaction(context.Background(), s.createAlice.TxID, nil)
	s.Require().NoError(err)
	_ = s.NotNil(r.Transaction) &&
		s.IsType((*CreateIdentity)(nil), r.Transaction.Body)
	_ = s.NotNil(r.Status) &&
		s.Equal(errors.Delivered, r.Status.Code)
	_ = s.NotNil(r.Produced) &&
		s.Len(r.Produced.Records, 1)
	_ = s.NotNil(r.Signatures) &&
		s.Len(r.Signatures.Records, 1)
}

func (s *QuerierTestSuite) TestQueryAccount() {
	r, err := s.QuerierFor(s.alice).QueryAccount(context.Background(), s.alice, nil)
	s.Require().NoError(err)
	_ = s.NotNil(r.Account) &&
		s.IsType((*ADI)(nil), r.Account)
	_ = s.NotNil(r.Directory) &&
		s.Len(r.Directory.Records, 2) &&
		s.True(r.Directory.Records[0].Value.Equal(s.alice.JoinPath("book")))
}

func (s *QuerierTestSuite) TestQueryChains() {
	r, err := s.QuerierFor(s.alice).QueryChains(context.Background(), s.alice, nil)
	s.Require().NoError(err)
	_ = s.Len(r.Records, 3) &&
		s.Equal("main", r.Records[0].Name) &&
		s.Equal(merkle.ChainTypeTransaction, r.Records[0].Type) &&
		s.Equal(2, int(r.Records[0].Count))
}

func (s *QuerierTestSuite) TestQueryChain() {
	r, err := s.QuerierFor(s.alice).QueryChain(context.Background(), s.alice, &api.ChainQuery{Name: "main"})
	s.Require().NoError(err)
	_ = s.Equal("main", r.Name) &&
		s.Equal(merkle.ChainTypeTransaction, r.Type) &&
		s.Equal(2, int(r.Count))
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
		s.IsType((*api.TransactionRecord)(nil), r.Value)
	})

	s.Run("ByValue", func() {
		r, err := s.QuerierFor(s.alice).QueryChainEntry(context.Background(), s.alice, &api.ChainQuery{Name: "main", Entry: hash[:]})
		s.Require().NoError(err)
		s.Equal(0, int(r.Index))
		s.IsType((*api.TransactionRecord)(nil), r.Value)
	})
}

func (s *QuerierTestSuite) TestQueryChainEntries() {
	r, err := s.QuerierFor(s.alice).QueryTxnChainEntries(context.Background(), s.alice, &api.ChainQuery{Name: "main", Range: &api.RangeOptions{}})
	s.Require().NoError(err)
	s.Require().Len(r.Records, 2)
}

func (s *QuerierTestSuite) TestQueryDataEntry() {
	entry := &AccumulateDataEntry{Data: [][]byte{[]byte("foo")}}

	s.Run("Latest", func() {
		r, err := s.QuerierFor(s.alice).QueryDataEntry(context.Background(), s.alice.JoinPath("data"), nil)
		s.Require().NoError(err)
		s.Equal(0, int(r.Index))
		s.Equal(entry.Hash(), r.Entry[:])
		s.IsType((*api.TransactionRecord)(nil), r.Value)
	})

	s.Run("ByIndex", func() {
		r, err := s.QuerierFor(s.alice).QueryDataEntry(context.Background(), s.alice.JoinPath("data"), &api.DataQuery{Index: uintp(0)})
		s.Require().NoError(err)
		s.Equal(entry.Hash(), r.Entry[:])
		s.IsType((*api.TransactionRecord)(nil), r.Value)
	})

	s.Run("ByValue", func() {
		r, err := s.QuerierFor(s.alice).QueryDataEntry(context.Background(), s.alice.JoinPath("data"), &api.DataQuery{Entry: entry.Hash()})
		s.Require().NoError(err)
		s.Equal(0, int(r.Index))
		s.IsType((*api.TransactionRecord)(nil), r.Value)
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
	s.Require().Len(r.Records, 2)
}

func (s *QuerierTestSuite) TestQueryPending() {
	r, err := s.QuerierFor(s.alice).QueryPending(context.Background(), s.alice, nil)
	s.Require().NoError(err)
	s.Require().Len(r.Records, 0)
}

func (s *QuerierTestSuite) TestQueryMinorBlock() {
	r, err := s.QuerierFor(s.alice).QueryMinorBlock(context.Background(), s.alice, &api.BlockQuery{Minor: uintp(2)})
	s.Require().NoError(err)
	s.Require().Len(r.Entries.Records, 1)
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
	s.Require().Len(r.Records, 1)
}

func (s *QuerierTestSuite) TestSearchForAnchor() {
	txr, err := s.QuerierFor(s.faucet).QueryTransaction(context.Background(), s.createAlice.TxID, nil)
	s.Require().NoError(err)
	txr, err = s.QuerierFor(s.alice).QueryTransaction(context.Background(), txr.Produced.Records[0].Value, nil)
	s.Require().NoError(err)

	var anchor []byte
	for _, r := range txr.Signatures.Records {
		for _, sig := range r.Signature.(*SignatureSet).Signatures {
			sig, ok := sig.(*ReceiptSignature)
			if ok {
				anchor = sig.Proof.Anchor
			}
		}
	}
	s.Require().NotNil(anchor)

	r, err := s.QuerierFor(DnUrl()).SearchForAnchor(context.Background(), DnUrl().JoinPath(AnchorPool), &api.AnchorSearchQuery{Anchor: anchor})
	s.Require().NoError(err)
	s.NotEmpty(r.Records)
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
	r, err := s.QuerierFor(s.faucet).SearchForTransactionHash(context.Background(), s.faucet, &api.TransactionHashSearchQuery{Hash: s.createAlice.TxID.Hash()})
	s.Require().NoError(err)
	s.Require().Len(r.Records, 1)
}
