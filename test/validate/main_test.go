// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package validate

import (
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/tendermint/tendermint/libs/log"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

var validateNetwork = flag.String("test.validate.network", "", "Validate a network")
var fullValidate = flag.Bool("test.validate.full", false, "Enable TestValidateFull")

// TestManualValidate is intended to be used to manually validate a deployed
// network.
func TestManualValidate(t *testing.T) {
	if *validateNetwork == "" {
		t.Skip()
	}
	addr, err := multiaddr.NewMultiaddr(*validateNetwork)
	require.NoError(t, err)
	suite.Run(t, &ValidationTestSuite{Network: []multiaddr.Multiaddr{addr}})
}

// TestValidate runs the validation test suite against the simulator.
func TestValidate(t *testing.T) {
	suite.Run(t, new(ValidationTestSuite))
}

// TestValidate runs the validation test suite against a full network.
func TestValidateFull(t *testing.T) {
	if !*fullValidate {
		t.Skip()
	}

	// Tendermint is stupid and doesn't properly shut down its databases. So we
	// have no way to ensure Tendermint is completely shut down, which leads to
	// race conditions. Instead of forking Tendermint or dumping a bunch of time
	// into some other solution, we're just going to leave Tendermint running.
	// That means this test cannot be run in the same process as any other test
	// that needs 127.0.1.X.
	dir, err := os.MkdirTemp("", "Accumulate-"+t.Name())
	require.NoError(t, err)
	// dir := t.TempDir()

	// Put the faucet on the DN
	faucet := AccountUrl("faucet")
	faucetKey := acctesting.GenerateKey(faucet)
	values := new(core.GlobalValues)
	values.Routing = new(RoutingTable)
	values.Routing.AddOverride(faucet, Directory)

	// Create a snapshot with the faucet
	db := database.OpenInMemory(nil)
	MakeIdentity(t, db, faucet, faucetKey[32:])
	UpdateAccount(t, db, faucet.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e12 })
	MakeAccount(t, db, &TokenAccount{Url: faucet.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e14)})
	batch := db.Begin(false)
	buf := new(ioutil2.Buffer)
	_, err = snapshot.Collect(batch, new(snapshot.Header), buf, snapshot.CollectOptions{})
	require.NoError(t, err)

	// Initialize the network configs
	netInit := simulator.LocalNetwork(t.Name(), 3, 3, net.ParseIP("127.0.1.1"), 30000)
	logger := acctesting.NewTestLogger(t)
	genDocs, err := accumulated.BuildGenesisDocs(netInit, values, time.Now(), logger, nil, []func() (ioutil2.SectionReader, error){
		func() (ioutil2.SectionReader, error) {
			return ioutil2.NewBuffer(buf.Bytes()), nil
		},
	})
	require.NoError(t, err)
	configs := accumulated.BuildNodesConfig(netInit, nil)

	newWriter := func(c *config.Config) (io.Writer, error) {
		// return logging.NewConsoleWriter(c.LogFormat)
		return logging.TestLogWriter(t)(c.LogFormat)
	}

	// Initialize the nodes
	var count int
	nodes := make([][][2]*accumulated.Daemon, len(configs))
	for i, configs := range configs {
		nodes[i] = make([][2]*accumulated.Daemon, len(configs))
		for j, configs := range configs {
			count++
			for k, cfg := range configs {
				// Use an in-memory database
				cfg.Accumulate.Storage.Type = config.MemoryStorage

				// Disable prometheus
				cfg.Instrumentation.Prometheus = false

				// Ignore Tendermint p2p errors
				cfg.LogLevel = config.LogLevel{}.
					Parse(config.DefaultLogLevels).
					SetModule("p2p", "fatal").
					String()

				// Set paths
				cfg.SetRoot(filepath.Join(dir, fmt.Sprintf("node-%d", count), cfg.Accumulate.PartitionId))

				// Write files so Tendermint can load them
				node := netInit.Bvns[i].Nodes[j]
				var nodeKey []byte
				if cfg.Accumulate.NetworkType == config.Directory {
					nodeKey = node.DnNodeKey
				} else {
					nodeKey = node.BvnNodeKey
				}
				err = accumulated.WriteNodeFiles(cfg, node.PrivValKey, nodeKey, genDocs[cfg.Accumulate.PartitionId])
				require.NoError(t, err)

				// Initialize the node
				nodes[i][j][k], err = accumulated.New(cfg, newWriter)
				require.NoError(t, err)
			}
		}
	}

	// // Don't complete until every node has been torn down
	// t.Cleanup(func() {
	// 	for _, nodes := range nodes {
	// 		for _, nodes := range nodes {
	// 			for _, node := range nodes {
	// 				<-node.Done()
	// 			}
	// 		}
	// 	}

	// 	// Give Tendermint a second to shut down
	// 	time.Sleep(10 * time.Second)
	// })

	// Start the nodes
	for _, nodes := range nodes {
		for _, nodes := range nodes {
			for _, node := range nodes {
				node := node
				require.NoError(t, node.Start())

				// Tendermint doesn't properly stop itself 😡
				// t.Cleanup(func() { _ = node.Stop() })
			}
		}
	}

	// Set up direct connections
	for _, nodes := range nodes {
		for _, nodes := range nodes {
			require.NoError(t, nodes[0].ConnectDirectly(nodes[1]))
		}
	}

	// Create the faucet service
	createFaucet(t, logger, faucetKey, configs[0][0][0].Accumulate.P2P.BootstrapPeers, faucet)

	color.HiBlack("----- Started -----")
	defer color.HiBlack("----- Stopping -----")
	suite.Run(t, &ValidationTestSuite{Network: []multiaddr.Multiaddr{nodes[0][0][0].P2P_TESTONLY().Addrs()[0]}})
}

func createFaucet(t *testing.T, logger log.Logger, faucetKey []byte, peers []multiaddr.Multiaddr, faucet *url.URL) {
	// Create the faucet node
	node, err := p2p.New(p2p.Options{
		Logger:         logger,
		BootstrapPeers: peers,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Close() })

	// Create the faucet service
	faucetSvc, err := v3impl.NewFaucet(context.Background(), v3impl.FaucetParams{
		Logger:    logger.With("module", "faucet"),
		Account:   faucet.JoinPath("tokens"),
		Key:       build.ED25519PrivateKey(faucetKey),
		Submitter: node,
		Querier:   node,
		Events:    node,
	})
	require.NoError(t, err)
	t.Cleanup(func() { faucetSvc.Stop() })

	// Register it
	handler, err := message.NewHandler(logger, message.Faucet{Faucet: faucetSvc})
	require.NoError(t, err)
	require.True(t, node.RegisterService(&api.ServiceAddress{Type: faucetSvc.Type()}, handler.Handle))
}

type ValidationTestSuite struct {
	Network []multiaddr.Multiaddr

	suite.Suite
	*Harness
	nonce     uint64
	faucetSvc api.Faucet
}

func (s *ValidationTestSuite) SetupSuite() {
	if s.Network == nil {
		s.setupForSim()
	} else {
		s.setupForNet()
	}
}

func (s *ValidationTestSuite) setupForSim() {
	// Set up the simulator and harness
	logger := acctesting.NewTestLogger(s.T())
	net := simulator.SimpleNetwork(s.T().Name(), 3, 1)
	sim, err := simulator.New(
		logger,
		simulator.MemoryDatabase,
		net,
		simulator.Genesis(GenesisTime),
	)
	s.Require().NoError(err)

	s.Harness = New(s.T(), sim.Services(), sim)

	// Set up the faucet
	faucet := AccountUrl("faucet")
	faucetKey := acctesting.GenerateKey(faucet)

	MakeIdentity(s.T(), sim.DatabaseFor(faucet), faucet, faucetKey[32:])
	UpdateAccount(s.T(), sim.DatabaseFor(faucet), faucet.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e12 })
	MakeAccount(s.T(), sim.DatabaseFor(faucet), &TokenAccount{Url: faucet.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e14)})

	faucetSvc, err := v3impl.NewFaucet(context.Background(), v3impl.FaucetParams{
		Logger:    logger.With("module", "faucet"),
		Account:   faucet.JoinPath("tokens"),
		Key:       build.ED25519PrivateKey(faucetKey),
		Submitter: sim.Services(),
		Querier:   sim.Services(),
		Events:    sim.Services(),
	})
	s.Require().NoError(err)
	s.T().Cleanup(func() { faucetSvc.Stop() })

	s.faucetSvc = faucetSvc
}

func (s *ValidationTestSuite) setupForNet() {
	// Set up the client
	s.T().Log("Create the client the harness")
	node, err := p2p.New(p2p.Options{
		Logger:         logging.ConsoleLoggerForTest(s.T(), "info"),
		BootstrapPeers: s.Network,
	})
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = node.Close() })

	s.T().Log("Wait for the faucet")
	node.WaitForService(&api.ServiceAddress{Type: api.ServiceTypeFaucet})

	// Set up the harness
	ctx, cancel := context.WithCancel(context.Background())
	s.T().Cleanup(cancel)
	events, err := node.Subscribe(ctx, api.SubscribeOptions{Partition: protocol.Directory})
	s.Require().NoError(err)
	s.Harness = New(s.T(), node, BlockStep(events))

	// Faucet via faucet
	s.faucetSvc = node
}

func (s *ValidationTestSuite) SetupTest() {
	s.Harness.TB = s.T()
}

func (s *ValidationTestSuite) faucet(account *url.URL) *protocol.TransactionStatus {
	sub, err := s.faucetSvc.Faucet(context.Background(), account, api.FaucetOptions{})
	s.Require().NoError(err)
	return sub.Status
}

func (s *ValidationTestSuite) TestMain() {
	// Set up lite addresses
	liteKey := acctesting.GenerateKey("Lite")
	liteAcme := acctesting.AcmeLiteAddressStdPriv(liteKey)
	liteId := liteAcme.RootIdentity()

	// Set up the ADI and its keys
	adi := AccountUrl("test")
	key10 := acctesting.GenerateKey(1, 0)
	key20 := acctesting.GenerateKey(2, 0)
	key21 := acctesting.GenerateKey(2, 1)
	key22 := acctesting.GenerateKey(2, 2)
	key23 := acctesting.GenerateKey(2, 3) // Orig
	key24 := acctesting.GenerateKey(2, 4) // New
	key30 := acctesting.GenerateKey(3, 0)
	key31 := acctesting.GenerateKey(3, 1)
	keymgr := acctesting.GenerateKey("mgr")

	ns := s.NetworkStatus(api.NetworkStatusOptions{Partition: protocol.Directory})
	oracle := float64(ns.Oracle.Price) / AcmeOraclePrecision

	s.TB.Log("Generate a Lite Token Account")
	st1 := s.faucet(liteAcme)
	st2 := s.faucet(liteAcme)
	s.StepUntil(
		Txn(st1.TxID).Succeeds(),
		Txn(st2.TxID).Succeeds(),
		Txn(st1.TxID).Produced().Succeeds(),
		Txn(st2.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*LiteTokenAccount](s.Harness, liteAcme).Balance)

	s.TB.Log("Add credits to lite account")
	st := s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(liteId).WithOracle(oracle).Purchase(1e6).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*LiteIdentity](s.Harness, liteId).CreditBalance)

	s.TB.Log("Create an ADI")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(adi).WithKey(key10, SignatureTypeED25519).WithKeyBook(adi, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	QueryAccountAs[*ADI](s.Harness, adi)

	s.TB.Log("Recreating an ADI fails and the synthetic transaction is recorded")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(adi).WithKey(key10, SignatureTypeED25519).WithKeyBook(adi, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Fails())

	s.TB.Log("Add credits to the ADI's key page 1")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(adi, "book", "1").WithOracle(oracle).Purchase(6e4).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "1")).CreditBalance)

	s.TB.Log("Create additional Key Pages")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book").
			CreateKeyPage().WithEntry().Key(key20, SignatureTypeED25519).FinishEntry().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2"))

	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book").
			CreateKeyPage().WithEntry().Key(key30, SignatureTypeED25519).FinishEntry().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "3"))

	s.TB.Log("Add credits to the ADI's key page 2")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(adi, "book", "2").WithOracle(oracle).Purchase(1e3).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).CreditBalance)

	s.TB.Log("Attempting to lock key page 2 using itself fails")
	st = s.BuildAndSubmit(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().UpdateAllowed().Deny(TransactionTypeUpdateKeyPage).FinishOperation().
			SignWith(adi, "book", "2").Version(1).Timestamp(&s.nonce).PrivateKey(key20))

	_ = s.NotNil(st.Error) &&
		s.Equal("signature 0: acc://test.acme/book/2 cannot modify its own allowed operations", st.Error.Message)

	s.Nil(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).TransactionBlacklist)

	s.TB.Log("Lock key page 2 using page 1")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().UpdateAllowed().Deny(TransactionTypeUpdateKeyPage).FinishOperation().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.NotNil(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).TransactionBlacklist)

	s.TB.Log("Attempting to update key page 3 using page 2 fails")
	st = s.BuildAndSubmit(
		build.Transaction().For(adi, "book", "3").
			UpdateKeyPage().Add().Entry().Key(key31, SignatureTypeED25519).FinishEntry().FinishOperation().
			SignWith(adi, "book", "2").Version(1).Timestamp(&s.nonce).PrivateKey(key20))

	_ = s.NotNil(st.Error) &&
		s.Equal("signature 0: page acc://test.acme/book/2 is not authorized to sign updateKeyPage", st.Error.Message)

	s.Len(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "3")).Keys, 1)

	s.TB.Log("Unlock key page 2 using page 1")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().UpdateAllowed().Allow(TransactionTypeUpdateKeyPage).FinishOperation().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Nil(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).TransactionBlacklist)

	s.TB.Log("Update key page 3 using page 2")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book", "3").
			UpdateKeyPage().Add().Entry().Key(key31, SignatureTypeED25519).FinishEntry().FinishOperation().
			SignWith(adi, "book", "2").Version(3).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Len(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "3")).Keys, 2)

	s.TB.Log("Add keys to page 2")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().
			Add().Entry().Key(key21, SignatureTypeED25519).FinishEntry().FinishOperation().
			Add().Entry().Key(key22, SignatureTypeED25519).FinishEntry().FinishOperation().
			Add().Entry().Key(key23, SignatureTypeED25519).FinishEntry().FinishOperation().
			SignWith(adi, "book", "2").Version(3).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Len(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).Keys, 4)

	s.TB.Log("Update key page entry with same keyhash different delegate")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateKeyBook(adi, "book2").WithKey(key20, SignatureTypeED25519).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book2", "1"))

	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(adi, "book2", "1").WithOracle(oracle).Purchase(1e3).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book2", "1")).CreditBalance)

	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book2", "1").
			UpdateKeyPage().Update().
			Entry().Key(key20, SignatureTypeED25519).FinishEntry().
			To().Key(key20, SignatureTypeED25519).Owner(adi, "book").FinishEntry().
			FinishOperation().
			SignWith(adi, "book2", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.NotNil(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book2", "1")).Keys[0].Delegate)

	s.TB.Log("Set KeyBook2 as authority for adi token account")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "acmetokens").ForToken(ACME).WithAuthority(adi, "book2").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book2", "1").Version(2).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Equal(adi.JoinPath("book2").String(), QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("acmetokens")).Authorities[0].Url.String())

	s.TB.Log("Burn Tokens (?) for adi token account")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			SendTokens(0.01, AcmePrecisionPower).To(adi, "acmetokens").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("acmetokens")).Balance)

	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "acmetokens").
			BurnTokens(0.01, AcmePrecisionPower).
			SignWith(adi, "book2", "1").Version(2).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Zero(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("acmetokens")).Balance)

	s.TB.Log("Set KeyBook2 as authority for adi data account")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "testdata1").ForToken(ACME).WithAuthority(adi, "book2").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book2", "1").Version(2).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Equal(adi.JoinPath("book2").String(), QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("testdata1")).Authorities[0].Url.String())

	_, _ = key24, keymgr

	s.TB.Log("Set threshold to 2 of 2")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().SetThreshold(2).
			SignWith(adi, "book", "2").Version(4).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Equal(2, int(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).AcceptThreshold))

	s.TB.Log("Set threshold to 0 of 0")
	st = s.BuildAndSubmit(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().SetThreshold(0).
			SignWith(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key20))

	_ = s.NotNil(st.Error) &&
		s.Equal("cannot require 0 signatures on a key page", st.Error.Message)

	s.TB.Log("Update a key with only that key's signature")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKey(key24, SignatureTypeED25519).
			SignWith(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key23))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	page := QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2"))
	doesNotHaveKey(s.T(), page, key23, SignatureTypeED25519)
	hasKey(s.T(), page, key24, SignatureTypeED25519)

	s.TB.Log("Create an ADI Token Account")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "tokens").ForToken(ACME).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("tokens"))

	s.TB.Log("Send tokens from the lite token account to the ADI token account")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			SendTokens(5, AcmePrecisionPower).To(adi, "tokens").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.Require().Equal("5.00000000", FormatBigAmount(&QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("tokens")).Balance, AcmePrecisionPower))

	s.TB.Log("Send tokens from the ADI token account to the lite token account using the multisig page")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "tokens").
			SendTokens(1, AcmePrecisionPower).To(liteAcme).
			SignWith(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	s.TB.Log("Signing the transaction with the same key does not deliver it")
	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	s.Require().NotZero(s.QueryPending(adi.JoinPath("tokens"), nil).Total)

	s.TB.Log("Sign the pending transaction using the other key")
	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key21))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.TB.Log("Signing the transaction after it has been delivered fails")
	st = s.BuildAndSubmit(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key22))

	h := st.TxID.Hash()
	_ = s.NotNil(st.Error) &&
		s.Equal(fmt.Sprintf("transaction %x (sendTokens) has been delivered", h[:4]), st.Error.Message)

	s.TB.Log("Create a token issuer")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateToken(adi, "token-issuer").WithSymbol("TOK").WithPrecision(10).WithSupplyLimit(1000000).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*TokenIssuer](s.Harness, adi.JoinPath("token-issuer"))

	s.TB.Log("Issue tokens")
	liteTok := liteId.JoinPath(adi.Authority, "token-issuer")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "token-issuer").
			IssueTokens("123.0123456789", 10).To(liteTok).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.Require().Equal("123.0123456789", FormatBigAmount(&QueryAccountAs[*LiteTokenAccount](s.Harness, liteTok).Balance, 10))

	s.TB.Log("Burn tokens")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteTok).
			BurnTokens(100, 10).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.Require().Equal("23.0123456789", FormatBigAmount(&QueryAccountAs[*LiteTokenAccount](s.Harness, liteTok).Balance, 10))

	s.TB.Log("Create lite data account and write the data")
	fde := &FactomDataEntry{ExtIds: [][]byte{[]byte("Factom PRO"), []byte("Tutorial")}}
	fde.AccountId = *(*[32]byte)(ComputeLiteDataAccountId(fde.Wrap()))
	lda, err := LiteDataAddress(fde.AccountId[:])
	s.Require().NoError(err)
	s.Require().Equal("acc://b36c1c4073305a41edc6353a094329c24ffa54c0a47fb56227a04477bcb78923", lda.String(), "Account ID is wrong")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(lda).
			WriteData().Entry(fde.Wrap()).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	st = s.QueryTransaction(st.TxID, nil).Status
	s.Require().IsType((*WriteDataResult)(nil), st.Result)
	wdr := st.Result.(*WriteDataResult)
	s.Require().Equal(lda.String(), wdr.AccountUrl.String())
	s.Require().NotZero(wdr.EntryHash)
	s.Require().NotEmpty(wdr.AccountID)

	s.TB.Log("Create ADI Data Account")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateDataAccount(adi, "data").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.TB.Log("Write data to ADI Data Account")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "data").
			WriteData([]byte("foo"), []byte("bar")).Scratch().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	st = s.QueryTransaction(st.TxID, nil).Status
	s.Require().IsType((*WriteDataResult)(nil), st.Result)
	wdr = st.Result.(*WriteDataResult)
	s.Require().NotZero(wdr.EntryHash)

	s.TB.Log("Create a sub ADI")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateIdentity(adi, "sub1").WithKeyBook(adi, "sub1", "book").WithKey(key10, SignatureTypeED25519).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.TB.Log("Create another ADI (manager)")
	manager := AccountUrl("manager")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(manager).WithKey(keymgr, SignatureTypeED25519).WithKeyBook(manager, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.TB.Log("Add credits to manager's key page 1")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(manager, "book", "1").WithOracle(oracle).Purchase(1e3).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.TB.Log("Create token account with manager")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "managed-tokens").ForToken(ACME).WithAuthority(adi, "book").WithAuthority(manager, "book").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTransaction(s.QueryTransaction(st.TxID, nil).Transaction).
			Url(manager, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(keymgr))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Require().Len(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("managed-tokens")).Authorities, 2)

	s.TB.Log("Remove manager from token account")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "managed-tokens").
			UpdateAccountAuth().Remove(manager, "book").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTransaction(s.QueryTransaction(st.TxID, nil).Transaction).
			Url(manager, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(keymgr))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Require().Len(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("managed-tokens")).Authorities, 1)

	s.TB.Log("Add manager to token account")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "managed-tokens").
			UpdateAccountAuth().Add(manager, "book").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTransaction(s.QueryTransaction(st.TxID, nil).Transaction).
			Url(manager, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(keymgr))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Require().Len(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("managed-tokens")).Authorities, 2)

	s.TB.Log("Transaction with Memo")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			Memo("hello world").
			SendTokens(1, AcmePrecisionPower).To(adi, "tokens").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.Require().Equal("hello world", s.QueryTransaction(st.TxID, nil).Transaction.Header.Memo)

	s.TB.Log("Refund on expensive synthetic txn failure")
	creditsBefore := QueryAccountAs[*LiteIdentity](s.Harness, liteId).CreditBalance
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(adi).WithKey(key10, SignatureTypeED25519).WithKeyBook(adi, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Fails(),
		Txn(st.TxID).Refund().Succeeds())
	creditsAfter := QueryAccountAs[*LiteIdentity](s.Harness, liteId).CreditBalance
	s.Require().Equal(100, int(creditsBefore-creditsAfter))

	tokensBefore := QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("tokens")).Balance.Int64()
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			SendTokens(5, AcmePrecisionPower).To("invalid-account").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Fails(),
		Txn(st.TxID).Refund().Succeeds())
	tokensAfter := QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("tokens")).Balance.Int64()
	s.Require().Equal(int(tokensBefore), int(tokensAfter))
}

func hasKey(tb testing.TB, page *KeyPage, key ed25519.PrivateKey, typ SignatureType) {
	tb.Helper()
	hash, err := PublicKeyHash(key[32:], typ)
	require.NoError(tb, err)
	_, _, ok := page.EntryByKeyHash(hash)
	require.Truef(tb, ok, "%v should have %x", page.Url, key[32:])
}

func doesNotHaveKey(tb testing.TB, page Signer2, key ed25519.PrivateKey, typ SignatureType) {
	tb.Helper()
	hash, err := PublicKeyHash(key[32:], typ)
	require.NoError(tb, err)
	_, _, ok := page.EntryByKeyHash(hash)
	require.Falsef(tb, ok, "%v should not have %x", page.GetUrl(), key[32:])
}
