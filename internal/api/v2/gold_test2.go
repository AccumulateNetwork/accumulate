// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build ignore
// +build ignore

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func (s *ValidationTestSuite) testApi(method string, req any) {
	s.T().Helper()
	var res json.RawMessage
	err := s.sim.API().RequestAPIv2(context.Background(), method, req, &res)
	s.Require().NoError(err)
	s.methods = append(s.methods, TestCall{Method: method, Request: req, Response: res})
}

type TestData struct {
	FinalHeight uint64                   `json:"finalHeight"`
	Network     *accumulated.NetworkInit `json:"network"`
	Genesis     map[string][]byte        `json:"genesis"`
	Steps       []*Step                  `json:"steps"`
	Cases       []TestCall               `json:"rpcCalls"`
}

type TestCall struct {
	Method   string          `json:"method"`
	Request  any             `json:"request"`
	Response json.RawMessage `json:"response"`
}

type Step struct {
	Block     uint64                          `json:"block"`
	Envelopes map[string][]*protocol.Envelope `json:"envelopes"`
	Snapshots map[string][]byte               `json:"snapshots"`
}

type ValidationTestSuite struct {
	suite.Suite
	*Harness
	sim   *simulator.Simulator
	nonce uint64

	methods []TestCall
	steps   []*Step
}

// Submit calls the envelope builder and submits the
// envelope. Submit fails if the transaction failed.
func (s *ValidationTestSuite) Submit(b EnvelopeBuilder, pending, produces bool) *TransactionStatus {
	s.T().Helper()

	env, err := b.Done()
	s.Require().NoError(err)
	s.Require().Len(env.Transaction, 1)

	status := s.SubmitSuccessfully(env)
	if status.Error != nil {
		s.Require().NoError(status.Error)
	}
	return status
}

func (s *ValidationTestSuite) StepUntil(conditions ...Condition) {
	s.T().Helper()
	for i := 0; ; i++ {
		if i >= 50 {
			s.T().Fatal("Condition not met after 50 blocks")
		}
		ok := true
		for _, c := range conditions {
			if !c(s.Harness) {
				ok = false
			}
		}
		if ok {
			break
		}

		s.Step()
	}
}

func (s *ValidationTestSuite) Step() {
	step := s.steps[len(s.steps)-1]
	step.Snapshots = map[string][]byte{}
	s.steps = append(s.steps, new(Step))

	s.Harness.Step()

	step.Block = s.sim.BlockIndex(Directory)

	for _, part := range s.sim.Partitions() {
		helpers.View(s.T(), s.sim.Database(part.ID), func(batch *database.Batch) {
			header := &snapshot.Header{
				Height:   step.Block,
				RootHash: *(*[32]byte)(batch.BptRoot()),
			}
			buf := new(ioutil2.Buffer)
			_, err := snapshot.Collect(batch, header, buf, snapshot.CollectOptions{
				PreserveAccountHistory: func(*database.Account) (bool, error) { return true, nil },
			})
			s.Require().NoError(err)
			step.Snapshots[part.ID] = buf.Bytes()
		})
	}
}

// StepN calls the stepper N times.
func (s *ValidationTestSuite) StepN(n int) {
	s.T().Helper()
	for i := 0; i < n; i++ {
		s.Step()
	}
}

func (s *ValidationTestSuite) TestMain() {
	var testData TestData
	s.steps = []*Step{{}}

	// Set up lite addresses
	liteKey := acctesting.GenerateKey("Lite")
	liteAcme := acctesting.AcmeLiteAddressStdPriv(liteKey)
	liteId := liteAcme.RootIdentity()

	// Use the simulator to create genesis documents
	net := simulator.SimpleNetwork("Gold/1.0.0", 3, 1)
	gsim, err := simulator.New(
		logging.NewTestLogger(s.T(), "plain", "error", false),
		simulator.MemoryDatabase,
		net,
		simulator.Genesis(GenesisTime),
	)
	s.Require().NoError(err)

	// var bvn0_genesis_root []byte
	// helpers.View(s.T(), gsim.Database("BVN0"), func(batch *database.Batch) {
	// 	u := PartitionUrl("BVN0").JoinPath(Ledger)
	// 	c, err := batch.Account(u).RootChain().Get()
	// 	s.Require().NoError(err)
	// 	bvn0_genesis_root = c.Anchor()
	// })

	helpers.MakeLiteTokenAccount(s.T(), gsim.DatabaseFor(liteId), liteKey[32:], AcmeUrl())
	helpers.CreditCredits(s.T(), gsim.DatabaseFor(liteId), liteId, 1e18)
	helpers.CreditTokens(s.T(), gsim.DatabaseFor(liteId), liteAcme, big.NewInt(1e17))

	// Step until block 7
	for gsim.BlockIndex(Directory) < 7 {
		s.Require().NoError(gsim.Step())
	}

	// Capture the snapshot now as the genesis snapshot
	testData.Genesis = map[string][]byte{}
	for _, part := range gsim.Partitions() {
		helpers.View(s.T(), gsim.Database(part.ID), func(batch *database.Batch) {
			buf := new(ioutil2.Buffer)
			_, err := snapshot.Collect(batch, &snapshot.Header{Height: testData.FinalHeight, RootHash: *(*[32]byte)(batch.BptRoot())}, buf, snapshot.CollectOptions{
				PreserveAccountHistory: func(*database.Account) (bool, error) { return true, nil },
			})
			s.Require().NoError(err)
			testData.Genesis[part.ID] = buf.Bytes()
		})
	}

	// Set up the simulator and harness
	sim, err := simulator.New(
		logging.NewTestLogger(s.T(), "plain", "error", false),
		simulator.MemoryDatabase,
		net,
		simulator.SnapshotMap(testData.Genesis),
	)
	s.Require().NoError(err)
	testData.Network = net

	for _, p := range sim.Partitions() {
		id := p.ID
		sim.SetSubmitHook(p.ID, func(m messaging.Message) (dropTx bool, keepHook bool) {
			l := m.(*messaging.LegacyMessage)
			step := s.steps[len(s.steps)-1]
			if step.Envelopes == nil {
				step.Envelopes = map[string][]*Envelope{}
			}
			step.Envelopes[id] = append(step.Envelopes[id], &Envelope{
				Transaction: []*Transaction{l.Transaction},
				Signatures:  l.Signatures,
			})
			return false, true
		})
	}

	s.sim = sim
	s.Harness = New(s.T(), sim.Services(), sim)

	fmt.Println(helpers.GetAccount[*AnchorLedger](s.T(), sim.Database("BVN1"), PartitionUrl("BVN1").JoinPath(AnchorPool)).MinorBlockSequenceNumber)

	// Set up the ADI and its keys
	adi := AccountUrl("test")
	key10 := acctesting.GenerateKey(1, 0)
	key20 := acctesting.GenerateKey(2, 0)
	key21 := acctesting.GenerateKey(2, 1)

	ns := s.NetworkStatus(api.NetworkStatusOptions{Partition: Directory})
	oracle := float64(ns.Oracle.Price) / AcmeOraclePrecision

	for _, part := range ns.Network.Partitions {
		partUrl := PartitionUrl(part.ID)
		helpers.View(s.T(), s.sim.Database(part.ID), func(batch *database.Batch) {
			var ledger *SystemLedger
			err := batch.Account(partUrl.JoinPath(Ledger)).Main().GetAs(&ledger)
			s.Require().NoError(err)

			buf := new(ioutil2.Buffer)
			err = snapshot.FullCollect(batch, buf, config.NetworkUrl{URL: partUrl}, acctesting.NewTestLogger(s.T()), true)
			s.Require().NoError(err)

			testData.Genesis[part.ID] = buf.Bytes()
		})
	}

	s.NotZero(QueryAccountAs[*LiteTokenAccount](s.Harness, liteAcme).Balance)

	st := s.Submit(
		build.Transaction().For(liteAcme).
			AddCredits().To(liteId).WithOracle(oracle).Purchase(1e6).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey), false, true)
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*LiteIdentity](s.Harness, liteId).CreditBalance)

	st = s.Submit(
		build.Transaction().For(liteAcme).
			CreateIdentity(adi).WithKey(key10, SignatureTypeED25519).WithKeyBook(adi, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey), false, true)
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	QueryAccountAs[*ADI](s.Harness, adi)

	st = s.Submit(
		build.Transaction().For(liteAcme).
			AddCredits().To(adi, "book", "1").WithOracle(oracle).Purchase(6e4).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey), false, true)
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "1")).CreditBalance)

	st = s.Submit(
		build.Transaction().For(adi, "book").
			CreateKeyPage().WithEntry().Key(key20, SignatureTypeED25519).FinishEntry().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10), false, false)
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2"))

	st = s.Submit(
		build.Transaction().For(liteAcme).
			AddCredits().To(adi, "book", "2").WithOracle(oracle).Purchase(1e3).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey), false, true)
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).CreditBalance)

	st = s.Submit(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().
			Add().Entry().Key(key21, SignatureTypeED25519).FinishEntry().FinishOperation().
			SignWith(adi, "book", "2").Version(1).Timestamp(&s.nonce).PrivateKey(key20), false, false)
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	/*
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 * **************************************************************************
	 */

	// txr := s.QueryTransaction(st.TxID, nil)
	// sig := txr.Signatures.Records[0].Signature.(*SignatureSet).Signatures[0]

	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: adi}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryOptions: v2.QueryOptions{Prove: true}})
	// s.testApi("query-directory", &v2.DirectoryQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}})
	// s.testApi("query-directory", &v2.DirectoryQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, QueryOptions: v2.QueryOptions{Expand: true}})
	// s.testApi("query-tx", &v2.TxnQuery{TxIdUrl: st.TxID})
	// s.testApi("query-tx-history", &v2.TxHistoryQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}})
	// s.testApi("query-data", &v2.DataEntryQuery{Url: adi.JoinPath("data")})
	// s.testApi("query-data-set", &v2.DataEntrySetQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("data")}, QueryPagination: v2.QueryPagination{Count: 10}})
	// s.testApi("query-data-set", &v2.DataEntrySetQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("data")}, QueryPagination: v2.QueryPagination{Count: 10}, QueryOptions: v2.QueryOptions{Expand: true}})
	// s.testApi("query-key-index", &v2.KeyPageIndexQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("book", "1")}, Key: key10[32:]})

	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithQuery(fmt.Sprintf("txid=%x", txr.TxID.Hash()))}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: txr.TxID.AsUrl()}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: txr.TxID.AsUrl()}, QueryOptions: v2.QueryOptions{Prove: true}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithQuery(fmt.Sprintf("txid=%x", sig.Hash()))}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithTxID(*(*[32]byte)(sig.Hash())).AsUrl()}})
	// s.testApi("query-tx", &v2.TxnQuery{Txid: sig.Hash()})

	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: DnUrl().JoinPath(AnchorPool).WithFragment(fmt.Sprintf("anchor/%x", bvn0_genesis_root))}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: pending.TxID.Account().WithFragment("pending")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: pending.TxID.Account().WithFragment("pending/0")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: pending.TxID.Account().WithFragment("pending/0:10")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: pending.TxID.Account().WithFragment(fmt.Sprintf("pending/%x", pending.TxID.Hash()))}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithQuery("count=10").WithFragment("transaction")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithQuery("count=10").WithFragment("signature")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithQuery("count=10").WithFragment("chain/main")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteAcme.WithFragment("chain/main/0")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteAcme.WithFragment(fmt.Sprintf("chain/main/%x", st.TxID.Hash()))}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteAcme.WithFragment("transaction/0")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteAcme.WithFragment(fmt.Sprintf("transaction/%x", st.TxID.Hash()))}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithFragment("signature/0")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithFragment(fmt.Sprintf("signature/%x", sig.Hash()))}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("data").WithFragment("data")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("data").WithFragment("data/0")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("data").WithFragment("data/0:10")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("data").WithFragment(fmt.Sprintf("data/%x", wdr.EntryHash))}})

	// s.testApi("query-major-blocks", &v2.MajorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}})
	// s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: v2.TxFetchModeCountOnly, BlockFilterMode: v2.BlockFilterModeExcludeEmpty})
	// s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: v2.TxFetchModeExpand, BlockFilterMode: v2.BlockFilterModeExcludeEmpty})
	// s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: v2.TxFetchModeIds, BlockFilterMode: v2.BlockFilterModeExcludeEmpty})
	// s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: v2.TxFetchModeOmit, BlockFilterMode: v2.BlockFilterModeExcludeEmpty})
	// s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: v2.TxFetchModeCountOnly, BlockFilterMode: v2.BlockFilterModeExcludeNone})
	// s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: v2.TxFetchModeExpand, BlockFilterMode: v2.BlockFilterModeExcludeNone})
	// s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: v2.TxFetchModeIds, BlockFilterMode: v2.BlockFilterModeExcludeNone})
	// s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: v2.TxFetchModeOmit, BlockFilterMode: v2.BlockFilterModeExcludeNone})

	// Step one last time to capture a snapshot at the end
	s.Step()

	testData.Steps = s.steps[:len(s.steps)-1]
	testData.Cases = s.methods
	testData.FinalHeight = s.sim.BlockIndex(Directory)

	f, err := os.Create("../../../test/testdata/api-v2-consistency.json")
	s.Require().NoError(err)
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(testData)
	s.Require().NoError(err)
}

// TestValidate runs the validation test suite against the simulator.
func TestValidate(t *testing.T) {
	suite.Run(t, new(ValidationTestSuite))
}
