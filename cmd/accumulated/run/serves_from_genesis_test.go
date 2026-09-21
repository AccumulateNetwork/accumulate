// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"testing"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A NODE THAT STARTED FROM GENESIS ANSWERS THE READS THE SPEC RESERVES FOR
// `COMPLETE` (#4368).
//
// executor.md, "Sync", step 6: "In this phase a syncing node refuses every
// read and answers once it is fully synced ... `BOOTING` and `ACTIVE` refuse
// with `NotReady`, `COMPLETE` serves." The two reads named there are the two
// a pull makes — a BPT page, and an account read carrying a receipt
// (`servingFor`, internal/api/v3/querier.go) — and this node answers both.
//
// This pins the fact both candidate resolutions of #4368 keep, and only that
// fact: whether the state such a node reports is called `ACTIVE` (the code
// today) or `COMPLETE` (the spec's predicate made reachable), a node that
// took nothing from a peer serves. It does not assert which state it is in;
// that is #4368's decision and it belongs to the spec.
//
// Nothing is built by hand. The network is the daemon's own start path, the
// client is the real JSON-RPC client over HTTP against the node's own API,
// and the querier is the one `(*Querier).start` registered with whatever
// `nodestate.Serving` `dagbft.go` gave it.
func TestAFromGenesisNodeAnswersTheTwoPullReads(t *testing.T) {
	api := startNetsimAndExecute(t)
	c := jsonrpc.NewClient(api)
	ctx := context.Background()

	// The BPT page: what a joining peer's enumerate.Stale reads, and the
	// first of the two `servingFor` refuses while a node is joining.
	for _, part := range []string{protocol.Directory, "BVN1"} {
		_, err := c.Query(ctx, protocol.PartitionUrl(part), &apiv3.BptPageQuery{Count: 4})
		require.NoError(t, err, "a from-genesis node refused to page %s's BPT", part)
	}

	// The account read with a receipt: what pull.Fetch reads, and the one a
	// peer's state is proven from.
	for _, part := range []string{protocol.Directory, "BVN1"} {
		u := protocol.PartitionUrl(part).JoinPath(protocol.Ledger)
		r, err := c.Query(ctx, u, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
		require.NoError(t, err, "a from-genesis node refused to prove %v", u)
		require.NotNil(t, r)
	}

	// And its state is on the wire for both partitions it runs, whatever the
	// value is: a reader that must decide whether this node can answer has
	// something to read (#4345a). The value itself is #4368's question.
	g := gaugeByPartition(t, "accumulate_node_state")
	require.Contains(t, g, "directory")
	require.Contains(t, g, "bvn1")
}

// A NODE OUTSIDE THE COMMITTEE, STARTED FROM GENESIS, ANSWERS READS AND
// RELAYS WHAT IT CANNOT PROPOSE (#4368, PLAN E11 second pass item 4).
//
// Two daemons in one process, each a CoreValidator through New and Start, as
// a Docker node is: a one-validator committee, and a second node whose
// validator key is in no committee, given the same genesis documents and the
// validator's libp2p address and nothing else. The second node has executed
// nothing of its own, so it does not join (a nil node-state machine) and it
// follows the committee from genesis, over gossip.
//
// The committee is not a netsim: a netsim's validators are sub-nodes, and a
// sub-node's DAG-BFT has no gossip (SubnodeService.start shares the host but
// not the pubsub router), so nothing outside the process could follow it.
// The netsim is used only to write the genesis documents and derive the
// validator and faucet keys; it is never started.
//
// The reads go to the querier the second node's daemon registered, not to a
// routed client that could be answered by the validator. The transaction goes
// to the second node's submit service, dialled by its peer ID, and is proven
// where it lands: the validator executed it. A relay is never proven by what
// the relaying node answered.
//
// What is by hand: the two configurations, and the libp2p addresses each is
// told of the other.
func TestAFromGenesisNodeOutsideTheCommitteeAnswersAndRelays(t *testing.T) {
	if testing.Short() {
		t.Skip("starts two daemons")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	const netName = "OutsideCommittee"
	logging := &Logging{Format: "plain", Rules: []*LoggingRule{{Level: slog.LevelError}}}
	autoServer := DhtMode(dht.ModeAutoServer)

	// Genesis: a one-validator, one-BVN network written by the netsim's own
	// generator, and the keys it derived for the validator and the faucet.
	genDir := t.TempDir()
	genCfg := &Config{
		Network: netName,
		P2P:     &P2P{Key: rawPrivKeyFrom(newEd25519Key(t))},
		Configurations: []Configuration{
			&NetSimConfiguration{
				Bvns:       1,
				Validators: 1,
				Globals: &network.GlobalValues{
					Globals: &protocol.NetworkGlobals{MajorBlockSchedule: "0 0 1 1 *"},
				},
			},
		},
	}
	genCfg.file = filepath.Join(genDir, "accumulate.toml")
	genInst := &Instance{rootDir: genDir, logger: slog.Default()}
	sim := genCfg.Configurations[0].(*NetSimConfiguration)
	require.NoError(t, sim.apply(genInst, genCfg))
	valKey, err := sim.generateKey(genInst, genCfg, 0, 0, "privVal")
	require.NoError(t, err)
	faucetKey, err := sim.generateKey(genInst, genCfg, "faucet")
	require.NoError(t, err)
	faucet, err := protocol.LiteTokenAddress(faucetKey[32:], "ACME", protocol.SignatureTypeED25519)
	require.NoError(t, err)

	// Each daemon's libp2p identity and address, known before either starts
	// so each can be told of the other.
	valNodeKey, outNodeKey := newEd25519Key(t), newEd25519Key(t)
	valP2P := multiaddr.StringCast(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", freeLocalPort(t)))
	outP2P := multiaddr.StringCast(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", freeLocalPort(t)))
	valPeer := addrForPeer(valP2P, peerIDOf(t, valNodeKey))
	outPeer := addrForPeer(outP2P, peerIDOf(t, outNodeKey))

	node := func(dir string, nodeKey, validatorKey []byte, self, other multiaddr.Multiaddr) *Instance {
		cfg := &Config{
			Network: netName,
			Logging: logging,
			P2P: &P2P{
				Key:            rawPrivKeyFrom(nodeKey),
				Listen:         []multiaddr.Multiaddr{self},
				BootstrapPeers: []multiaddr.Multiaddr{other},
				DiscoveryMode:  &autoServer,
			},
			Configurations: []Configuration{
				&CoreValidatorConfiguration{
					Mode:         CoreValidatorModeDual,
					BVN:          "BVN1",
					Listen:       multiaddr.StringCast(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", freeDevnetBase(t, 1))),
					ValidatorKey: rawPrivKeyFrom(validatorKey),
					DnGenesis:    filepath.Join(genDir, "dn-genesis.snap"),
					BvnGenesis:   filepath.Join(genDir, "bvn1-genesis.snap"),
				},
			},
		}
		cfg.file = filepath.Join(dir, "accumulate.toml")
		inst, err := New(ctx, cfg)
		require.NoError(t, err)
		require.NoError(t, inst.Start())
		t.Cleanup(inst.Stop)
		return inst
	}

	// The committee, then the node outside it. The second node's ports are
	// picked only once the first node holds its own.
	val := node(t.TempDir(), valNodeKey, valKey, valP2P, outPeer)
	out := node(t.TempDir(), outNodeKey, newEd25519Key(t), outP2P, valPeer)

	// The second node's own querier, as its daemon registered it.
	q, err := querierProvides.Get(out.services, &Querier{Partition: "BVN1"})
	require.NoError(t, err)
	ledgerUrl := protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)
	ownLedger := func() uint64 {
		r, err := q.Query(ctx, ledgerUrl, &apiv3.DefaultQuery{})
		if err != nil {
			return 0
		}
		ar, ok := r.(*apiv3.AccountRecord)
		if !ok {
			return 0
		}
		l, ok := ar.Account.(*protocol.SystemLedger)
		if !ok {
			return 0
		}
		return l.Index
	}

	// It follows: it executes blocks the committee made, which it can only
	// have from the committee.
	require.Eventually(t, func() bool { return ownLedger() > protocol.GenesisBlock+5 },
		90*time.Second, time.Second, "the node outside the committee executed nothing past genesis")

	// It answers reads, plain and the two a pull makes.
	_, err = q.Query(ctx, faucet, &apiv3.DefaultQuery{})
	require.NoError(t, err, "a from-genesis node outside the committee refused a plain read")
	_, err = q.Query(ctx, protocol.PartitionUrl("BVN1"), &apiv3.BptPageQuery{Count: 4})
	require.NoError(t, err, "a from-genesis node outside the committee refused to page its BPT")
	_, err = q.Query(ctx, ledgerUrl, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
	require.NoError(t, err, "a from-genesis node outside the committee refused to prove its ledger")

	// It relays. A client with no services of its own dials the second
	// node's submit service by peer ID.
	client, err := p2p.New(p2p.Options{
		Network:        netName,
		Listen:         []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/127.0.0.1/tcp/0")},
		BootstrapPeers: []multiaddr.Multiaddr{outPeer},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	peerClient := &message.Client{Transport: &message.RoutedTransport{
		Network: netName,
		Dialer:  client.DialNetwork(),
	}}

	recipientKey := newEd25519Key(t)
	recipient, err := protocol.LiteTokenAddress(recipientKey[32:], "ACME", protocol.SignatureTypeED25519)
	require.NoError(t, err)
	env, err := build.Transaction().For(faucet).
		SendTokens(1, protocol.AcmePrecisionPower).To(recipient).
		SignWith(faucet.RootIdentity()).Version(1).Timestamp(uint64(time.Now().UnixMicro())).
		Type(protocol.SignatureTypeED25519).PrivateKey(faucetKey).Done()
	require.NoError(t, err)

	taken0 := testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues("BVN1", metrics.RelayTaken))
	require.Eventually(t, func() bool {
		_, err = peerClient.ForPeer(out.p2p.ID()).
			ForAddress(apiv3.ServiceTypeSubmit.AddressFor("BVN1").Multiaddr()).
			Submit(ctx, env, apiv3.SubmitOptions{})
		return err == nil
	}, 30*time.Second, time.Second, "the node outside the committee did not take the submission")
	require.NoError(t, err)

	// Proven at the destination: the validator executed it. Read through
	// the validator's own querier, not the second node's.
	valQ, err := querierProvides.Get(val.services, &Querier{Partition: "BVN1"})
	require.NoError(t, err)
	var balance uint64
	require.Eventually(t, func() bool {
		r, err := valQ.Query(ctx, recipient, &apiv3.DefaultQuery{})
		if err != nil {
			return false
		}
		ar, ok := r.(*apiv3.AccountRecord)
		if !ok {
			return false
		}
		lta, ok := ar.Account.(*protocol.LiteTokenAccount)
		if !ok {
			return false
		}
		balance = lta.Balance.Uint64()
		return true
	}, 60*time.Second, time.Second, "the committee never executed the transaction the node outside it relayed")
	require.Equal(t, uint64(protocol.AcmePrecision), balance)

	// And it went by relay, not by a proposal of the second node's own: the
	// only node in this process that cannot propose is the second one.
	require.Greater(t, testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues("BVN1", metrics.RelayTaken)), taken0,
		"the submission reached the committee without being relayed")
}

// newEd25519Key returns a fresh ed25519 private key.
func newEd25519Key(t *testing.T) []byte {
	t.Helper()
	_, sk, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return sk
}

// peerIDOf returns the libp2p peer ID of an ed25519 node key.
func peerIDOf(t *testing.T, sk []byte) peer.ID {
	t.Helper()
	pub, err := libp2pcrypto.UnmarshalEd25519PublicKey(sk[32:])
	require.NoError(t, err)
	id, err := peer.IDFromPublicKey(pub)
	require.NoError(t, err)
	return id
}

// freeLocalPort returns a TCP port on 127.0.0.1 that was free a moment ago.
func freeLocalPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
