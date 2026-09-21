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
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
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
// Two daemons in one process, each through New and Start: a one-validator
// netsim, and a second node whose validator key is in no committee, given the
// netsim's genesis documents and its libp2p address and nothing else. The
// second node has executed nothing of its own, so it does not join (a nil
// node-state machine) and it follows the committee from genesis.
//
// The reads go to the querier the second node's daemon registered, not to a
// routed client that could be answered by the validator. The transaction goes
// to the second node's submit service, dialled by its peer ID, and is proven
// where it lands: the validator executed it. A relay is never proven by what
// the relaying node answered.
//
// What is by hand: the second node's configuration (a CoreValidator with a
// key outside the definition, which is what a Docker follower is), and the
// libp2p bootstrap address, which the netsim does not publish.
func TestAFromGenesisNodeOutsideTheCommitteeAnswersAndRelays(t *testing.T) {
	if testing.Short() {
		t.Skip("starts two daemons")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	const netName = "OutsideCommittee"
	logging := &Logging{Format: "plain", Rules: []*LoggingRule{{Level: slog.LevelError}}}
	autoServer := DhtMode(dht.ModeAutoServer)

	// The committee: one validator, on a libp2p address the second node can
	// be told.
	valDir := t.TempDir()
	valBase := freeDevnetBase(t, 1)
	valP2P := multiaddr.StringCast(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", freeLocalPort(t)))
	valCfg := &Config{
		Network: netName,
		Logging: logging,
		P2P: &P2P{
			Key:           &PrivateKeySeed{Seed: record.NewKey("outside-committee-validator")},
			Listen:        []multiaddr.Multiaddr{valP2P},
			DiscoveryMode: &autoServer,
		},
		Configurations: []Configuration{
			&NetSimConfiguration{
				Listen:     multiaddr.StringCast(fmt.Sprintf("/tcp/%d", valBase)),
				Bvns:       1,
				Validators: 1,
				Globals: &network.GlobalValues{
					Globals: &protocol.NetworkGlobals{MajorBlockSchedule: "0 0 1 1 *"},
				},
			},
		},
	}
	valCfg.file = filepath.Join(valDir, "accumulate.toml")
	val, err := New(ctx, valCfg)
	require.NoError(t, err)
	val.rootDir = valDir

	// The faucet: a lite account the netsim's genesis funded, with the key
	// the netsim derived for it.
	sim := valCfg.Configurations[0].(*NetSimConfiguration)
	faucetKey, err := sim.generateKey(val, valCfg, "faucet")
	require.NoError(t, err)
	faucet, err := protocol.LiteTokenAddress(faucetKey[32:], "ACME", protocol.SignatureTypeED25519)
	require.NoError(t, err)

	// The node outside the committee.
	_, outsideKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	outDir := t.TempDir()
	outCfg := &Config{
		Network: netName,
		Logging: logging,
		P2P: &P2P{
			Key:            &PrivateKeySeed{Seed: record.NewKey("outside-committee-node")},
			DiscoveryMode:  &autoServer,
			BootstrapPeers: nil, // set once the validator's peer ID is known
		},
		Configurations: []Configuration{
			&CoreValidatorConfiguration{
				Mode:         CoreValidatorModeDual,
				BVN:          "BVN1",
				Listen:       nil, // set once the validator's ports are taken
				ValidatorKey: rawPrivKeyFrom(outsideKey),
				DnGenesis:    filepath.Join(valDir, "dn-genesis.snap"),
				BvnGenesis:   filepath.Join(valDir, "bvn1-genesis.snap"),
			},
		},
	}
	outCfg.file = filepath.Join(outDir, "accumulate.toml")

	require.NoError(t, val.Start())
	t.Cleanup(val.Stop)
	outCfg.P2P.BootstrapPeers = []multiaddr.Multiaddr{addrForPeer(valP2P, val.p2p.ID())}
	outCfg.Configurations[0].(*CoreValidatorConfiguration).Listen =
		multiaddr.StringCast(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", freeDevnetBase(t, 1)))

	out, err := New(ctx, outCfg)
	require.NoError(t, err)
	out.rootDir = outDir
	require.NoError(t, out.Start())
	t.Cleanup(out.Stop)

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
		BootstrapPeers: []multiaddr.Multiaddr{addrForPeer(valP2P, val.p2p.ID())},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	peerClient := &message.Client{Transport: &message.RoutedTransport{
		Network: netName,
		Dialer:  client.DialNetwork(),
	}}

	_, recipientKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
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
	var balance string
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
		balance = lta.Balance.String()
		return true
	}, 60*time.Second, time.Second, "the committee never executed the transaction the node outside it relayed")
	require.Equal(t, fmt.Sprint(protocol.AcmePrecision), balance)

	// And it went by relay, not by a proposal of the second node's own: the
	// only node in this process that cannot propose is the second one.
	require.Greater(t, testutil.ToFloat64(metrics.RelayedTotal.WithLabelValues("BVN1", metrics.RelayTaken)), taken0,
		"the submission reached the committee without being relayed")
}

// freeLocalPort returns a TCP port on 127.0.0.1 that was free a moment ago.
func freeLocalPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
