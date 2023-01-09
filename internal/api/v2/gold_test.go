// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	v2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2/query"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func (s *ValidationTestSuite) testApi(method string, req any) {
	var res json.RawMessage
	err := s.sim.API().RequestAPIv2(context.Background(), method, req, &res)
	s.Require().NoError(err)
	s.methods = append(s.methods, TestCall{Method: method, Request: req, Response: res})
}

type TestCall struct {
	Method   string          `json:"method"`
	Request  any             `json:"request"`
	Response json.RawMessage `json:"response"`
}

type Submission struct {
	Block    uint64    `json:"block"`
	Envelope *Envelope `json:"envelope"`
	Pending  bool      `json:"pending"`
	Produces bool      `json:"produces"`
}

type ValidationTestSuite struct {
	suite.Suite
	*Harness
	sim   *simulator.Simulator
	nonce uint64

	methods     []TestCall
	submissions []Submission
}

// Submit calls the envelope builder and submits the
// envelope. Submit fails if the transaction failed.
func (s *ValidationTestSuite) Submit(b EnvelopeBuilder, pending, produces bool) *TransactionStatus {
	s.T().Helper()
	env, err := b.Done()
	s.Require().NoError(err)
	s.Require().Len(env.Transaction, 1)
	d, err := chain.NormalizeEnvelope(env)
	s.Require().NoError(err)
	s.Require().Len(d, 1)

	s.submissions = append(s.submissions, Submission{
		Block:    s.sim.BlockIndex(Directory),
		Envelope: env,
		Pending:  pending,
		Produces: produces,
	})

	status := s.SubmitSuccessfully(d[0])
	if status.Error != nil {
		s.Require().NoError(status.Error)
	}
	return status
}

func (s *ValidationTestSuite) TestMain() {
	testData := struct {
		FinalHeight uint64                   `json:"finalHeight"`
		Network     *accumulated.NetworkInit `json:"network"`
		Genesis     map[string][]byte        `json:"genesis"`
		Roots       map[string][]byte        `json:"roots"`
		Submissions []Submission             `json:"submissions"`
		Cases       []TestCall               `json:"rpcCalls"`
	}{
		Genesis: map[string][]byte{},
		Roots:   map[string][]byte{},
	}

	// Set up lite addresses
	liteKey := acctesting.GenerateKey("Lite")
	liteAcme := acctesting.AcmeLiteAddressStdPriv(liteKey)
	liteId := liteAcme.RootIdentity()

	mdb := database.OpenInMemory(nil)
	helpers.MakeLiteTokenAccount(s.T(), mdb, liteKey[32:], AcmeUrl())
	helpers.CreditCredits(s.T(), mdb, liteId, 1e18)
	helpers.CreditTokens(s.T(), mdb, liteAcme, big.NewInt(1e17))
	var liteSnap []byte
	helpers.View(s.T(), mdb, func(batch *database.Batch) {
		buf := new(ioutil2.Buffer)
		_, err := snapshot.Collect(batch, new(snapshot.Header), buf, snapshot.CollectOptions{
			PreserveAccountHistory: func(account *database.Account) (bool, error) { return true, nil },
		})
		s.Require().NoError(err)
		liteSnap = buf.Bytes()
	})

	values := new(core.GlobalValues)
	values.Globals = new(NetworkGlobals)
	values.Globals.MajorBlockSchedule = "*/10 * * * *"

	var genDocs map[string]*tmtypes.GenesisDoc
	var genesis simulator.SnapshotFunc = func(partition string, network *accumulated.NetworkInit, logger log.Logger) (ioutil2.SectionReader, error) {
		var err error
		if genDocs == nil {
			genDocs, err = accumulated.BuildGenesisDocs(network, values, GenesisTime, logger, nil, []func() (ioutil2.SectionReader, error){func() (ioutil2.SectionReader, error) { return ioutil2.NewBuffer(liteSnap), nil }})
			if err != nil {
				return nil, errors.UnknownError.WithFormat("build genesis docs: %w", err)
			}
		}

		var snapshot []byte
		err = json.Unmarshal(genDocs[partition].AppState, &snapshot)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		return ioutil2.NewBuffer(snapshot), nil
	}

	// Set up the simulator and harness
	net := simulator.SimpleNetwork("Gold/1.0.0", 3, 1)
	sim, err := simulator.New(
		logging.NewTestLogger(s.T(), "plain", "error", false),
		simulator.MemoryDatabase,
		net,
		genesis,
	)
	s.Require().NoError(err)
	testData.Network = net

	s.sim = sim
	s.Harness = New(s.T(), sim.Services(), sim)

	var bvn0_genesis_root []byte
	helpers.View(s.T(), s.sim.Database("BVN0"), func(batch *database.Batch) {
		var ledger *SystemLedger
		record := batch.Account(PartitionUrl("BVN0").JoinPath(Ledger))
		s.Require().NoError(record.Main().GetAs(&ledger))
		s.Require().Equal(1, int(ledger.Index))
		c, err := record.RootChain().Get()
		s.Require().NoError(err)
		bvn0_genesis_root = c.Anchor()
	})

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

	st = s.Submit(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().SetThreshold(2).
			SignWith(adi, "book", "2").Version(2).Timestamp(&s.nonce).PrivateKey(key20), false, false)
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Equal(2, int(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).AcceptThreshold))

	s.StepN(int(10 * time.Minute / time.Second))

	st = s.Submit(
		build.Transaction().For(adi).
			CreateDataAccount(adi, "data").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10), false, false)
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	st = s.Submit(
		build.Transaction().For(adi, "data").
			WriteData([]byte("foo"), []byte("bar")).Scratch().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10), false, false)
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	st = s.QueryTransaction(st.TxID, nil).Status
	s.Require().IsType((*WriteDataResult)(nil), st.Result)
	wdr := st.Result.(*WriteDataResult)
	s.Require().NotZero(wdr.EntryHash)

	st = s.Submit(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "tokens").ForToken(ACME).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10), false, false)
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("tokens"))

	st = s.Submit(
		build.Transaction().For(liteAcme).
			SendTokens(5, AcmePrecisionPower).To(adi, "tokens").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey), false, true)
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// pending := s.Submit(
	// 	build.Transaction().For(adi, "tokens").
	// 		SendTokens(1, AcmePrecisionPower).To(liteAcme).
	// 		SignWith(adi, "book", "2").Version(3).Timestamp(&s.nonce).PrivateKey(key20), true, false)
	// s.StepUntil(
	// 	Txn(pending.TxID).IsPending())

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

	txr := s.QueryTransaction(st.TxID, nil)
	sig := txr.Signatures.Records[0].Signature.(*SignatureSet).Signatures[0]

	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: adi}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryOptions: v2.QueryOptions{Prove: true}})
	s.testApi("query-directory", &v2.DirectoryQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}})
	s.testApi("query-directory", &v2.DirectoryQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, QueryOptions: v2.QueryOptions{Expand: true}})
	s.testApi("query-tx", &v2.TxnQuery{TxIdUrl: st.TxID})
	s.testApi("query-tx-history", &v2.TxHistoryQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}})
	s.testApi("query-data", &v2.DataEntryQuery{Url: adi.JoinPath("data")})
	s.testApi("query-data-set", &v2.DataEntrySetQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("data")}, QueryPagination: v2.QueryPagination{Count: 10}})
	s.testApi("query-data-set", &v2.DataEntrySetQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("data")}, QueryPagination: v2.QueryPagination{Count: 10}, QueryOptions: v2.QueryOptions{Expand: true}})
	s.testApi("query-key-index", &v2.KeyPageIndexQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("book", "1")}, Key: key10[32:]})

	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithQuery(fmt.Sprintf("txid=%x", txr.TxID.Hash()))}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: txr.TxID.AsUrl()}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: txr.TxID.AsUrl()}, QueryOptions: v2.QueryOptions{Prove: true}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithQuery(fmt.Sprintf("txid=%x", sig.Hash()))}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithTxID(*(*[32]byte)(sig.Hash())).AsUrl()}})
	s.testApi("query-tx", &v2.TxnQuery{Txid: sig.Hash()})

	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: DnUrl().JoinPath(AnchorPool).WithFragment(fmt.Sprintf("anchor/%x", bvn0_genesis_root))}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: pending.TxID.Account().WithFragment("pending")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: pending.TxID.Account().WithFragment("pending/0")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: pending.TxID.Account().WithFragment("pending/0:10")}})
	// s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: pending.TxID.Account().WithFragment(fmt.Sprintf("pending/%x", pending.TxID.Hash()))}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithQuery("count=10").WithFragment("transaction")}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithQuery("count=10").WithFragment("signature")}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithQuery("count=10").WithFragment("chain/main")}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteAcme.WithFragment("chain/main/0")}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteAcme.WithFragment(fmt.Sprintf("chain/main/%x", st.TxID.Hash()))}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteAcme.WithFragment("transaction/0")}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteAcme.WithFragment(fmt.Sprintf("transaction/%x", st.TxID.Hash()))}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithFragment("signature/0")}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: liteId.WithFragment(fmt.Sprintf("signature/%x", sig.Hash()))}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("data").WithFragment("data")}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("data").WithFragment("data/0")}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("data").WithFragment("data/0:10")}})
	s.testApi("query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: adi.JoinPath("data").WithFragment(fmt.Sprintf("data/%x", wdr.EntryHash))}})

	s.testApi("query-major-blocks", &v2.MajorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}})
	s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: query.TxFetchModeCountOnly, BlockFilterMode: query.BlockFilterModeExcludeEmpty})
	s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: query.TxFetchModeExpand, BlockFilterMode: query.BlockFilterModeExcludeEmpty})
	s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: query.TxFetchModeIds, BlockFilterMode: query.BlockFilterModeExcludeEmpty})
	s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: query.TxFetchModeOmit, BlockFilterMode: query.BlockFilterModeExcludeEmpty})
	s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: query.TxFetchModeCountOnly, BlockFilterMode: query.BlockFilterModeExcludeNone})
	s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: query.TxFetchModeExpand, BlockFilterMode: query.BlockFilterModeExcludeNone})
	s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: query.TxFetchModeIds, BlockFilterMode: query.BlockFilterModeExcludeNone})
	s.testApi("query-minor-blocks", &v2.MinorBlocksQuery{UrlQuery: v2.UrlQuery{Url: adi}, QueryPagination: v2.QueryPagination{Count: 10}, TxFetchMode: query.TxFetchModeOmit, BlockFilterMode: query.BlockFilterModeExcludeNone})

	testData.Submissions = s.submissions
	testData.Cases = s.methods
	testData.FinalHeight = s.sim.BlockIndex(Directory)

	for _, part := range ns.Network.Partitions {
		helpers.View(s.T(), s.sim.Database(part.ID), func(batch *database.Batch) {
			testData.Roots[part.ID] = batch.BptRoot()
		})
	}

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
