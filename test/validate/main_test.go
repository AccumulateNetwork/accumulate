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
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	tmp2p "github.com/tendermint/tendermint/p2p"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

var validateNetwork = flag.String("test.validate.network", "", "Validate a network")

func init() {
	acctesting.EnableDebugFeatures()
}

// TestValidate runs the validation test suite against the simulator.
func TestValidate(t *testing.T) {
	suite.Run(t, new(ValidationTestSuite))
}

// TestValidateAPI runs the validation test suite against the simulator via API
// v2 over P2P.
func TestValidateAPI(t *testing.T) {
	acctesting.SkipCI(t, "Not sufficiently reliable yet")

	net := simulator.LocalNetwork(t.Name(), 3, 1, net.ParseIP("127.0.1.1"), 12345)

	// Set up the simulator
	s := new(ValidationTestSuite)
	s.sim, s.faucetSvc = setupSim(t, net)

	// Start listening
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := s.sim.ListenAndServe(ctx, simulator.ListenOptions{
		ListenP2Pv3: true,
	})
	require.NoError(t, err)

	// Set up the P2P client node
	t.Log("Create the client")
	logger := logging.ConsoleLoggerForTest(t, "info")
	node, err := p2p.New(p2p.Options{
		Network: net.Id,
		Logger:  logger,
		BootstrapPeers: []multiaddr.Multiaddr{
			net.Bvns[0].Nodes[0].Listen().Scheme("tcp").Directory().AccumulateP2P().WithKey().Multiaddr(),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Close() })

	// Run the faucet through the API
	handler, err := message.NewHandler(logger, message.Faucet{Faucet: s.faucetSvc})
	require.NoError(t, err)
	node.RegisterService(api.ServiceTypeFaucet.AddressForUrl(protocol.AcmeUrl()), handler.Handle)

	// Wait for the nodes to get connected
	waitFor(t, node, net.Id, api.ServiceTypeSubmit.AddressFor(Directory), time.Minute)
	for _, b := range net.Bvns {
		waitFor(t, node, net.Id, api.ServiceTypeSubmit.AddressFor(b.Id), time.Minute)
	}

	// Create a harness that uses the P2P client node for services but steps the
	// simulator directly
	s.Harness = New(s.T(), node, s.sim)
	s.node, s.faucetSvc = node, node

	suite.Run(t, s)
}

// TestValidateNetwork is intended to be used to manually validate a deployed
// network.
func TestValidateNetwork(t *testing.T) {
	if *validateNetwork == "" {
		t.Skip()
	}

	var bootstrap []multiaddr.Multiaddr
	var network string
	if addr, err := multiaddr.NewMultiaddr(*validateNetwork); err == nil { //nolint
		t.Fatalf("Not supported - we have to figure out how to get the network")
		bootstrap = append(bootstrap, addr)
	} else {
		if st, err := os.Stat(*validateNetwork); err != nil || !st.IsDir() {
			t.Fatalf("%q is neither an address nor a node directory", *validateNetwork)
		}

		// Load the node and derive its listening address
		node, err := accumulated.Load(*validateNetwork, nil)
		require.NoError(t, err)
		network = node.Config.Accumulate.Network.Id
		key, err := tmp2p.LoadNodeKey(node.Config.NodeKeyFile())
		require.NoError(t, err)
		ed := ed25519.PrivateKey(key.PrivKey.Bytes())
		sk, _, err := crypto.KeyPairFromStdKey(&ed)
		require.NoError(t, err)
		id, err := peer.IDFromPrivateKey(sk)
		require.NoError(t, err)
		c, err := multiaddr.NewComponent("p2p", id.String())
		require.NoError(t, err)
		for _, addr := range node.Config.Accumulate.P2P.Listen {
			bootstrap = append(bootstrap, addr.Encapsulate(c))
		}
	}

	harness, node := setupNetClient(t, network, bootstrap...)
	waitFor(t, node, network, api.ServiceTypeFaucet.Address(), time.Minute)

	s := new(ValidationTestSuite)
	s.Harness, s.faucetSvc, s.node = harness, node, node
	suite.Run(t, s)
}

func waitFor(t *testing.T, node *p2p.Node, network string, sa *api.ServiceAddress, timeout time.Duration) {
	ma, err := sa.MultiaddrFor(network)
	require.NoError(t, err)

	fmt.Printf("Wait for %v\n", sa)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err = node.WaitForService(ctx, ma)
	require.NoError(t, err, "%v did not appear within %v", sa, timeout)
}

type ValidationTestSuite struct {
	suite.Suite
	*Harness
	sim       *simulator.Simulator
	nonce     uint64
	faucetSvc api.Faucet
	node      *p2p.Node
}

func (s *ValidationTestSuite) SetupSuite() {
	if s.Harness != nil {
		return
	}

	s.sim, s.faucetSvc = setupSim(s.T(), simulator.SimpleNetwork(s.T().Name(), 3, 1))
	s.Harness = New(s.T(), s.sim.Services(), s.sim)
}

func setupSim(t *testing.T, net *accumulated.NetworkInit) (*simulator.Simulator, api.Faucet) {
	// Set up the simulator and harness
	logger := acctesting.NewTestLogger(t)
	sim, err := simulator.New(
		logger,
		simulator.MemoryDatabase,
		net,
		simulator.Genesis(GenesisTime),
	)
	require.NoError(t, err)

	// Set up the faucet
	faucet := AccountUrl("faucet")
	faucetKey := acctesting.GenerateKey(faucet)

	MakeIdentity(t, sim.DatabaseFor(faucet), faucet, faucetKey[32:])
	UpdateAccount(t, sim.DatabaseFor(faucet), faucet.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e12 })
	MakeAccount(t, sim.DatabaseFor(faucet), &TokenAccount{Url: faucet.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e14)})

	faucetSvc, err := v3impl.NewFaucet(context.Background(), v3impl.FaucetParams{
		Logger:    logger.With("module", "faucet"),
		Account:   faucet.JoinPath("tokens"),
		Key:       build.ED25519PrivateKey(faucetKey),
		Submitter: sim.Services(),
		Querier:   sim.Services(),
		Events:    sim.Services(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { faucetSvc.Stop() })

	return sim, faucetSvc
}

func setupNetClient(t *testing.T, network string, addrs ...multiaddr.Multiaddr) (*Harness, *p2p.Node) {
	// Set up the client
	t.Log("Create the client")
	node, err := p2p.New(p2p.Options{
		Network:        network,
		Logger:         logging.ConsoleLoggerForTest(t, "info"),
		BootstrapPeers: addrs,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Close() })

	// Set up the harness
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	events, err := node.Subscribe(ctx, api.SubscribeOptions{Partition: protocol.Directory})
	require.NoError(t, err)

	h := New(t, node, BlockStep(events))
	return h, node
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
	s.TB.Skip()
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
	st := s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(liteId).WithOracle(oracle).Purchase(1e6).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*LiteIdentity](s.Harness, liteId).CreditBalance)

	s.TB.Log("Burn credits")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteId).
			BurnCredits(1).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.TB.Log("Create an ADI")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(adi).WithKey(key10, SignatureTypeED25519).WithKeyBook(adi, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	QueryAccountAs[*ADI](s.Harness, adi)

	s.TB.Log("Recreating an ADI fails and the synthetic transaction is recorded")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(adi).WithKey(key10, SignatureTypeED25519).WithKeyBook(adi, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Fails())

	s.TB.Log("Add credits to the ADI's key page 1")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(adi, "book", "1").WithOracle(oracle).Purchase(6e4).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "1")).CreditBalance)

	s.TB.Log("Create additional Key Pages")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "book").
			CreateKeyPage().WithEntry().Key(key20, SignatureTypeED25519).FinishEntry().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2"))

	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "book").
			CreateKeyPage().WithEntry().Key(key30, SignatureTypeED25519).FinishEntry().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "3"))

	s.TB.Log("Add credits to the ADI's key page 2")
	st = s.BuildAndSubmitTxnSuccessfully(
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
			SignWith(adi, "book", "2").Version(1).Timestamp(&s.nonce).PrivateKey(key20))[1]

	s.EqualError(st.AsError(), "acc://test.acme/book/2 cannot modify its own allowed operations")

	s.Nil(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).TransactionBlacklist)

	s.TB.Log("Lock key page 2 using page 1")
	st = s.BuildAndSubmitTxnSuccessfully(
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
			SignWith(adi, "book", "2").Version(1).Timestamp(&s.nonce).PrivateKey(key20))[1]

	s.EqualError(st.AsError(), "page acc://test.acme/book/2 is not authorized to sign updateKeyPage")

	s.Len(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "3")).Keys, 1)

	s.TB.Log("Unlock key page 2 using page 1")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().UpdateAllowed().Allow(TransactionTypeUpdateKeyPage).FinishOperation().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Nil(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).TransactionBlacklist)

	s.TB.Log("Update key page 3 using page 2")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "book", "3").
			UpdateKeyPage().Add().Entry().Key(key31, SignatureTypeED25519).FinishEntry().FinishOperation().
			SignWith(adi, "book", "2").Version(3).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Len(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "3")).Keys, 2)

	s.TB.Log("Add keys to page 2")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "book", "2").
			Memo("foo").
			UpdateKeyPage().
			Add().Entry().Key(key21, SignatureTypeED25519).FinishEntry().FinishOperation().
			Add().Entry().Key(key22, SignatureTypeED25519).FinishEntry().FinishOperation().
			Add().Entry().Key(key23, SignatureTypeED25519).FinishEntry().FinishOperation().
			SignWith(adi, "book", "2").Version(3).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Len(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).Keys, 4)

	s.TB.Log("Update key page entry with same keyhash different delegate")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi).
			CreateKeyBook(adi, "book2").WithKey(key20, SignatureTypeED25519).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book2", "1"))

	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(adi, "book2", "1").WithOracle(oracle).Purchase(1e3).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book2", "1")).CreditBalance)

	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "book2", "1").
			UpdateKeyPage().Update().
			Entry().Key(key20, SignatureTypeED25519).FinishEntry().
			To().Key(key20, SignatureTypeED25519).Owner(adi, "book").FinishEntry().
			FinishOperation().
			SignWith(adi, "book2", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.NotNil(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book2", "1")).Keys[0].Delegate)

	// Stop early if the -short flag is specified
	if testing.Short() {
		return
	}

	s.TB.Log("Set KeyBook2 as authority for adi token account")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "acmetokens").ForToken(ACME).WithAuthority(adi, "book2").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book2", "1").Version(2).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Equal(adi.JoinPath("book2").String(), QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("acmetokens")).Authorities[0].Url.String())

	s.TB.Log("Burn Tokens (?) for adi token account")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			SendTokens(0.01, AcmePrecisionPower).To(adi, "acmetokens").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("acmetokens")).Balance)

	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "acmetokens").
			BurnTokens(0.01, AcmePrecisionPower).
			SignWith(adi, "book2", "1").Version(2).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Zero(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("acmetokens")).Balance)

	s.TB.Log("Set KeyBook2 as authority for adi data account")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "testdata1").ForToken(ACME).WithAuthority(adi, "book2").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book2", "1").Version(2).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Equal(adi.JoinPath("book2").String(), QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("testdata1")).Authorities[0].Url.String())

	_, _ = key24, keymgr

	s.TB.Log("Set threshold to 2 of 2")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().SetThreshold(2).
			SignWith(adi, "book", "2").Version(4).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Equal(2, int(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).AcceptThreshold))

	s.TB.Log("Set threshold to 0 of 0")
	st = s.BuildAndSubmitTxn(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().SetThreshold(0).
			SignWith(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key20))

	s.EqualError(st.AsError(), "cannot require 0 signatures on a key page")

	s.TB.Log("Update a key with only that key's signature")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKey(key24, SignatureTypeED25519).
			SignWith(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key23))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	page := QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2"))
	doesNotHaveKey(s.T(), page, key23, SignatureTypeED25519)
	hasKey(s.T(), page, key24, SignatureTypeED25519)

	s.TB.Log("Create an ADI Token Account")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "tokens").ForToken(ACME).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("tokens"))

	s.TB.Log("Send tokens from the lite token account to the ADI token account")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			SendTokens(5, AcmePrecisionPower).To(adi, "tokens").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.Require().Equal("5.00000000", FormatBigAmount(&QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("tokens")).Balance, AcmePrecisionPower))

	s.TB.Log("Send tokens from the ADI token account to the lite token account using the multisig page")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "tokens").
			SendTokens(1, AcmePrecisionPower).To(liteAcme).
			SignWith(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	s.TB.Log("Signing the transaction with the same key does not deliver it")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	s.Require().NotZero(s.QueryPending(adi.JoinPath("tokens"), nil).Total)

	s.TB.Log("Sign the pending transaction using the other key")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key21))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.TB.Log("Signing the transaction after it has been delivered fails")
	st = s.BuildAndSubmitTxn(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key22))
	s.Equal(errors.Delivered, st.Code)

	var dropped *url.TxID
	if s.sim != nil {
		s.TB.Log("Drop the next anchor")
		s.sim.SetBlockHook(Directory, func(_ execute.BlockParams, messages []messaging.Message) (_ []messaging.Message, keepHook bool) {
			// Drop all block anchors, once
			for i := 0; i < len(messages); i++ {
				if anchor, ok := messages[i].(*messaging.BlockAnchor); ok {
					messages = append(messages[:i], messages[i+1:]...)
					dropped = anchor.Anchor.(*messaging.SequencedMessage).Message.ID()
				}
			}
			return messages, dropped == nil
		})

		defer func() {
			// Wait for an anchor to be dropped
			s.StepUntil(True(func(*Harness) bool { return dropped != nil }))

			// Wait for that anchor to be healed
			s.StepUntil(Txn(dropped).Succeeds())
		}()
	}

	s.TB.Log("Create a token issuer")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi).
			CreateToken(adi, "token-issuer").WithSymbol("TOK").WithPrecision(10).WithSupplyLimit(1000000).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*TokenIssuer](s.Harness, adi.JoinPath("token-issuer"))

	s.TB.Log("Issue tokens")
	liteTok := liteId.JoinPath(adi.Authority, "token-issuer")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "token-issuer").
			IssueTokens("123.0123456789", 10).To(liteTok).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.Require().Equal("123.0123456789", FormatBigAmount(&QueryAccountAs[*LiteTokenAccount](s.Harness, liteTok).Balance, 10))

	s.TB.Log("Burn tokens")
	st = s.BuildAndSubmitTxnSuccessfully(
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
	st = s.BuildAndSubmitTxnSuccessfully(
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
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi).
			CreateDataAccount(adi, "data").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.TB.Log("Write data to ADI Data Account")
	st = s.BuildAndSubmitTxnSuccessfully(
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
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi).
			CreateIdentity(adi, "sub1").WithKeyBook(adi, "sub1", "book").WithKey(key10, SignatureTypeED25519).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.TB.Log("Create another ADI (manager)")
	manager := AccountUrl("manager")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(manager).WithKey(keymgr, SignatureTypeED25519).WithKeyBook(manager, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.TB.Log("Add credits to manager's key page 1")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(manager, "book", "1").WithOracle(oracle).Purchase(1e3).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.TB.Log("Create token account with manager")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "managed-tokens").ForToken(ACME).WithAuthority(adi, "book").WithAuthority(manager, "book").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(s.QueryTransaction(st.TxID, nil).Transaction).
			Url(manager, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(keymgr))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Require().Len(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("managed-tokens")).Authorities, 2)

	s.TB.Log("Remove manager from token account")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "managed-tokens").
			UpdateAccountAuth().Remove(manager, "book").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(s.QueryTransaction(st.TxID, nil).Transaction).
			Url(manager, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(keymgr))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Require().Len(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("managed-tokens")).Authorities, 1)

	s.TB.Log("Add manager to token account")
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(adi, "managed-tokens").
			UpdateAccountAuth().Add(manager, "book").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(s.QueryTransaction(st.TxID, nil).Transaction).
			Url(manager, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(keymgr))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Require().Len(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("managed-tokens")).Authorities, 2)

	s.TB.Log("Transaction with Memo")
	st = s.BuildAndSubmitTxnSuccessfully(
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
	st = s.BuildAndSubmitTxnSuccessfully(
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
	st = s.BuildAndSubmitTxnSuccessfully(
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

func (s *ValidationTestSuite) TestFaucets() {
	if s.node == nil {
		s.T().Skip()
	}

	// The normal faucet works
	liteKey := acctesting.GenerateKey(s.T().Name(), "lite")
	liteAcme := acctesting.AcmeLiteAddressStdPriv(liteKey)
	liteId := liteAcme.RootIdentity()
	fst, err := s.node.Faucet(context.Background(), liteAcme, api.FaucetOptions{})
	s.Require().NoError(err)
	s.StepUntil(
		Txn(fst.Status.TxID).Succeeds(),
		Txn(fst.Status.TxID).Produced().Succeeds())

	// A random faucet fails
	_, err = s.node.Faucet(context.Background(), AccountUrl("foo"), api.FaucetOptions{Token: AccountUrl("foo", "tokens")})
	s.Require().Error(err)
	s.Require().IsType((*errors.Error)(nil), err)
	e := err.(*errors.Error)
	for e.Cause != nil {
		e = e.Cause
	}
	s.Require().EqualError(e, "no live peers for faucet:foo.acme!_tokens")

	// Set up a token issuer
	pegnet := AccountUrl("pegnet")
	pegKey := acctesting.GenerateKey(s.T().Name(), pegnet)
	ns := s.NetworkStatus(api.NetworkStatusOptions{Partition: protocol.Directory})
	oracle := float64(ns.Oracle.Price) / AcmeOraclePrecision

	st := s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(liteId).WithOracle(oracle).Purchase(1e6).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(pegnet).WithKey(pegKey, SignatureTypeED25519).WithKeyBook(pegnet, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(pegnet, "book", "1").WithOracle(oracle).Purchase(1e6).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(pegnet).
			CreateToken(pegnet, "peg").
			WithSymbol("PEG").
			SignWith(pegnet, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(pegKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	// Set up a new faucet
	logger := logging.ConsoleLoggerForTest(s.T(), "info")
	peg := pegnet.JoinPath("peg")
	faucetSvc, err := v3impl.NewFaucet(context.Background(), v3impl.FaucetParams{
		Logger:    logger,
		Account:   peg,
		Key:       build.ED25519PrivateKey(pegKey),
		Submitter: s.sim.Services(),
		Querier:   s.sim.Services(),
		Events:    s.sim.Services(),
	})
	s.Require().NoError(err)
	s.T().Cleanup(func() { faucetSvc.Stop() })

	handler, err := message.NewHandler(logger, message.Faucet{Faucet: faucetSvc})
	s.Require().NoError(err)
	s.node.RegisterService(api.ServiceTypeFaucet.AddressForUrl(peg), handler.Handle)

	// Use the new faucet
	litePeg := liteId.JoinPath(peg.ShortString())
	fst, err = s.node.Faucet(context.Background(), litePeg, api.FaucetOptions{})
	s.Require().NoError(err)
	s.StepUntil(
		Txn(fst.Status.TxID).Succeeds(),
		Txn(fst.Status.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*LiteTokenAccount](s.Harness, litePeg).Balance.Uint64())
}

func (s *ValidationTestSuite) TestNodeService() {
	if s.node == nil {
		s.T().Skip()
	}

	info, err := s.node.NodeInfo(context.Background(), api.NodeInfoOptions{})
	s.Require().NoError(err)
	s.Require().NotEmpty(info.Network)

	nodes, err := s.node.FindService(context.Background(), api.FindServiceOptions{Network: info.Network})
	s.Require().NoError(err)
	for _, n := range nodes {
		info, err := s.node.NodeInfo(context.Background(), api.NodeInfoOptions{PeerID: n.PeerID})
		s.Require().NoError(err)
		fmt.Printf("%v has %d service(s)\n", info.PeerID, len(info.Services))
		for _, svc := range info.Services {
			fmt.Printf("  %v\n", svc)
		}
	}
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
