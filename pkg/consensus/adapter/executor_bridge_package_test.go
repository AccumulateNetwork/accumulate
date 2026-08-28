// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"context"
	"io"
	"log/slog"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	multiexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	dagconfig "gitlab.com/accumulatenetwork/accumulate/pkg/consensus/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// #4141 sender side, through the real ProduceBlock seam: a destination's
// synthetic messages leave as packages — one envelope, one SyntheticProof
// covering exactly its contents — sized to fit the transport's batch limit.

// capDispatcher records every submitted envelope by destination.
type capDispatcher struct {
	mu   sync.Mutex
	sent []capSent
}

type capSent struct {
	dest *url.URL
	env  *messaging.Envelope
}

func (c *capDispatcher) Submit(_ context.Context, dest *url.URL, env *messaging.Envelope) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, capSent{dest, env})
	return nil
}
func (c *capDispatcher) Send(context.Context) <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}
func (c *capDispatcher) Close() {}

func (c *capDispatcher) take() []capSent {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sent
	c.sent = nil
	return s
}

// A BVN dispatches its synthetics only after the DN's anchor round-trip,
// which this harness does not have. The DIRECTORY dispatches on its own
// loopback anchor with no receipts required — so the sender-side packaging
// tests run a Directory-partition bridge and hand-deliver its own anchor.
// stubRouter routes partition URLs to their partitions, mapped authorities
// to their assigned partition, and everything else to the default — so the
// harness can put lite signers ON the Directory while their produced
// synthetics leave for the BVNs.
type stubRouter struct {
	assign map[string]string
	def    string
}

func (r stubRouter) RouteAccount(u *url.URL) (string, error) {
	if id, ok := protocol.ParsePartitionUrl(u); ok {
		return id, nil
	}
	if p, ok := r.assign[u.RootIdentity().Authority]; ok {
		return p, nil
	}
	return r.def, nil
}

func (r stubRouter) Route(envs ...*messaging.Envelope) (string, error) {
	return r.def, nil
}

type dnBridge struct {
	*realBridge
	globals *network.GlobalValues
	valKeys [][]byte // every DN validator, for anchor signature thresholds
	cap     *capDispatcher
}

func newDnBridge(t *testing.T, bvns int, maxEnv int, liteKeys ...[]byte) *dnBridge {
	t.Helper()
	netInit := simulator.NewSimpleNetwork("RealBridge", bvns, 1)
	genesisTime := time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC)
	values := new(core.GlobalValues)
	values.ExecutorVersion = protocol.ExecutorVersionLatest

	logger := logging.NewSlogLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	docs, err := accumulated.BuildGenesisDocs(netInit, values, genesisTime, logger, nil, nil)
	require.NoError(t, err)

	db := database.OpenInMemory(nil)
	require.NoError(t, snapshot.FullRestore(db, ioutil2.NewBuffer(docs[protocol.Directory]), nil, config.NetworkUrl{URL: protocol.DnUrl()}))

	// Fund lite accounts directly. Their identities would route to a BVN on a
	// real network; the harness executes them here regardless, because what
	// is under test is the SENDER side — remote synthetics leaving the DN.
	require.NoError(t, db.Update(func(batch *database.Batch) error {
		for _, key := range liteKeys {
			lite := protocol.LiteAuthorityForKey(key[32:], protocol.SignatureTypeED25519)
			err := batch.Account(lite).Main().Put(&protocol.LiteIdentity{Url: lite, CreditBalance: 1e9})
			if err != nil {
				return err
			}
			lta := lite.JoinPath("ACME")
			err = batch.Account(lta).Main().Put(&protocol.LiteTokenAccount{Url: lta, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(1e15)})
			if err != nil {
				return err
			}
		}
		return nil
	}))

	bus := events.NewBus(nil)
	// Lite signers live on the Directory in this harness; produced synthetics
	// leave for the BVNs.
	assign := map[string]string{
		"bvn0-bound.acme": "BVN0",
		"bvn1-bound.acme": "BVN1",
	}
	for _, key := range liteKeys {
		lite := protocol.LiteAuthorityForKey(key[32:], protocol.SignatureTypeED25519)
		assign[lite.Authority] = protocol.Directory
	}
	router := stubRouter{assign: assign, def: "BVN0"}
	cap_ := new(capDispatcher)

	var globals *network.GlobalValues
	events.SubscribeSync(bus, func(e events.WillChangeGlobals) error {
		globals = e.New
		return nil
	})

	var valKeys [][]byte
	for _, bvn := range netInit.Bvns {
		for _, node := range bvn.Nodes {
			valKeys = append(valKeys, node.PrivValKey)
		}
	}
	valKey := valKeys[0]
	exec, err := multiexec.NewExecutor(multiexec.Options{
		Database:        db,
		Key:             valKey,
		Router:          router,
		EventBus:        bus,
		NewDispatcher:   func() multiexec.Dispatcher { return cap_ },
		MaxEnvelopeSize: maxEnv,
		Describe: multiexec.DescribeShim{
			NetworkType: protocol.PartitionTypeDirectory,
			PartitionId: protocol.Directory,
		},
	})
	require.NoError(t, err)
	_, err = exec.Init(nil)
	require.NoError(t, err)
	require.NotNil(t, globals, "Init must publish globals")

	params, _, err := exec.LastBlock()
	require.NoError(t, err)
	bridge, err := NewExecutorBridge(ExecutorBridgeConfig{Executor: exec, PartitionID: protocol.Directory, EventBus: bus})
	require.NoError(t, err)

	r := &realBridge{bridge: bridge, db: db, index: params.Index, time: genesisTime.Add(time.Duration(params.Index) * time.Second)}
	return &dnBridge{realBridge: r, globals: globals, valKeys: valKeys, cap: cap_}
}

// deliverOwnAnchorAndCollect constructs the DN's anchor for the LAST block,
// delivers it to the DN itself (the loopback every real network performs),
// and produces blocks until the previous block's synthetics dispatch.
func (r *dnBridge) deliverOwnAnchorAndCollect(t *testing.T) []capSent {
	t.Helper()

	batch := r.db.Begin(false)
	body, _, err := crosschain.ConstructLastAnchor(context.Background(), batch, protocol.DnUrl())
	require.NoError(t, err)
	require.NotNil(t, body, "the last block must have prepared an anchor")

	// Use the sequence number the DESTINATION expects. The ledger's own
	// counter is ahead: earlier blocks (genesis included) prepared anchors
	// that no conductor ever delivered in this harness, and an out-of-order
	// anchor parks as pending instead of executing.
	var anchors *protocol.AnchorLedger
	require.NoError(t, batch.Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).Main().GetAs(&anchors))
	seqNum := anchors.Partition(protocol.DnUrl()).Delivered + 1
	batch.Discard()

	// One signed anchor envelope per DN validator, so the signature
	// threshold is met regardless of the network's size.
	var txs [][]byte
	for _, key := range r.valKeys {
		env, _, err := crosschain.ValidatorContext{
			Source:       &protocol.PartitionInfo{ID: protocol.Directory, Type: protocol.PartitionTypeDirectory},
			Globals:      r.globals,
			ValidatorKey: key,
		}.PrepareAnchorSubmission(context.Background(), body, seqNum, protocol.DnUrl())
		require.NoError(t, err)
		b, err := env.MarshalBinary()
		require.NoError(t, err)
		txs = append(txs, b)
	}
	_, err = r.produce(t, types.NewBatch(txs))
	require.NoError(t, err)
	r.cap.take() // drop anything dispatched before the anchor landed

	for i := 0; i < 3; i++ {
		_, err := r.produce(t)
		require.NoError(t, err)
		if sent := r.cap.take(); len(sent) > 0 {
			return sent
		}
	}
	t.Fatal("nothing was dispatched")
	return nil
}

// remoteBurns builds one batch of transfers from each key to a remote
// destination on BVN0. Delivery is not under test — the SENDER-side
// packaging is.
func remoteBurns(t *testing.T, keys [][]byte, ts uint64) *types.Batch {
	t.Helper()
	var txs [][]byte
	for i, key := range keys {
		from := protocol.LiteAuthorityForKey(key[32:], protocol.SignatureTypeED25519)
		env, err := build.Transaction().For(from.JoinPath("ACME")).
			SendTokens(uint64(i+1), 0).To(protocol.AccountUrl("remote-void", from.Hostname())).
			SignWith(from).Version(1).Timestamp(ts).PrivateKey(key).
			Done()
		require.NoError(t, err)
		b, err := env.MarshalBinary()
		require.NoError(t, err)
		txs = append(txs, b)
	}
	return types.NewBatch(txs)
}

func TestBridge_PackagesRemoteSyntheticsWithOneProof(t *testing.T) {
	keys := make([][]byte, 6)
	for i := range keys {
		keys[i] = acctesting.GenerateKey("pkg", i)
	}
	r := newDnBridge(t, 1, 0, keys...)

	_, err := r.produce(t, remoteBurns(t, keys, 1))
	require.NoError(t, err)
	sent := r.deliverOwnAnchorAndCollect(t)

	// One of the dispatches is the block anchor the DN sends; the package is
	// the envelope carrying synthetic messages.
	var pkgs []capSent
	for _, s := range sent {
		if _, ok := s.env.Messages[0].(*messaging.SyntheticProof); ok {
			pkgs = append(pkgs, s)
		}
	}
	for _, x := range sent {
		t.Logf("sent to %v: %d messages, first %v", x.dest, len(x.env.Messages), x.env.Messages[0].Type())
	}
	require.Len(t, pkgs, 1, "six messages for one destination leave as ONE package")
	env := pkgs[0].env
	require.True(t, pkgs[0].dest.Equal(protocol.PartitionUrl("BVN0")))

	// The proof leads, so a reader sees it before the messages that need it.
	proof, ok := env.Messages[0].(*messaging.SyntheticProof)
	require.True(t, ok, "the first message is the package's proof")
	require.NotNil(t, proof.Proof.ReceiptList)
	require.True(t, proof.Proof.ReceiptList.Validate(nil), "the proof must be valid")

	// Every synthetic in the package omits its own proof and is covered by
	// the package's.
	var members int
	for _, msg := range env.Messages[1:] {
		syn, ok := msg.(*messaging.SyntheticMessage)
		if !ok {
			continue // companions ride along as plain messages
		}
		members++
		assert.Nil(t, syn.Proof, "package members carry no proof of their own")
		h := syn.Message.Hash()
		assert.True(t, proof.Proof.ReceiptList.Included(h[:]),
			"the package's proof covers exactly its own contents")
	}
	assert.Equal(t, 6, members)

	// The whole envelope fits in one worker batch — the assertion that would
	// have caught main's 3 MiB budget against this transport's 500 KB limit.
	b, err := env.MarshalBinary()
	require.NoError(t, err)
	assert.LessOrEqual(t, len(b), dagconfig.DefaultMaxBatchBytes,
		"a package is one consensus transaction and must fit in one batch")
}

func TestBridge_SingletonGroupUsesIndividualReceipt(t *testing.T) {
	key := acctesting.GenerateKey("pkg", "solo")
	r := newDnBridge(t, 1, 0, key)

	_, err := r.produce(t, remoteBurns(t, [][]byte{key}, 1))
	require.NoError(t, err)
	sent := r.deliverOwnAnchorAndCollect(t)

	var synths []*messaging.Envelope
	for _, s := range sent {
		if _, ok := s.env.Messages[0].(*messaging.BlockAnchor); ok {
			continue // the DN's own anchor dispatch
		}
		synths = append(synths, s.env)
	}
	require.Len(t, synths, 1)
	env := synths[0]
	syn, ok := env.Messages[0].(*messaging.SyntheticMessage)
	require.True(t, ok, "no SyntheticProof for a singleton — a one-element list is larger than the receipt it replaces")
	require.NotNil(t, syn.Proof)
	assert.NotNil(t, syn.Proof.Receipt, "the singleton carries its own individual receipt")
	assert.Nil(t, syn.Proof.ReceiptList)
}

func TestBridge_GroupsByDestination(t *testing.T) {
	keys := make([][]byte, 4)
	for i := range keys {
		keys[i] = acctesting.GenerateKey("pkg", "grp", i)
	}
	// Two BVNs, so the DN's synthetics split across two remote destinations.
	// Where a lite identity routes depends on its hash; generate destinations
	// until both partitions are covered.
	r := newDnBridge(t, 2, 0, keys...)

	var txs [][]byte
	for i, key := range keys {
		from := protocol.LiteAuthorityForKey(key[32:], protocol.SignatureTypeED25519)
		dest := protocol.AccountUrl("bvn0-bound", "tokens")
		if i%2 == 0 {
			dest = protocol.AccountUrl("bvn1-bound", "tokens")
		}
		env, err := build.Transaction().For(from.JoinPath("ACME")).
			SendTokens(uint64(i+1), 0).To(dest).
			SignWith(from).Version(1).Timestamp(1).PrivateKey(key).
			Done()
		require.NoError(t, err)
		b, err := env.MarshalBinary()
		require.NoError(t, err)
		txs = append(txs, b)
	}
	_, err := r.produce(t, types.NewBatch(txs))
	require.NoError(t, err)
	sent := r.deliverOwnAnchorAndCollect(t)

	byDest := map[string]int{}
	for _, s := range sent {
		if _, ok := s.env.Messages[0].(*messaging.BlockAnchor); ok {
			continue // the DN's own anchor dispatches
		}
		byDest[s.dest.String()]++
		// One package never mixes destinations — every synthetic inside is
		// bound where the envelope is.
		for _, msg := range s.env.Messages {
			if syn, ok := msg.(*messaging.SyntheticMessage); ok {
				seq := syn.Message.(*messaging.SequencedMessage)
				assert.True(t, seq.Destination.Equal(s.dest))
			}
		}
	}
	assert.Len(t, byDest, 2, "two destinations, two groups: %v", byDest)
}

// With a budget too small for even one message, each goes alone rather than
// wedging the loop — visible one-message packages, not silence.
func TestBridge_OversizedSingleMessageGoesAloneRatherThanWedgingTheLoop(t *testing.T) {
	keys := make([][]byte, 3)
	for i := range keys {
		keys[i] = acctesting.GenerateKey("pkg", "big", i)
	}
	r := newDnBridge(t, 1, 64, keys...)

	_, err := r.produce(t, remoteBurns(t, keys, 1))
	require.NoError(t, err)
	sent := r.deliverOwnAnchorAndCollect(t)

	var pkgs int
	for _, s := range sent {
		if _, ok := s.env.Messages[0].(*messaging.BlockAnchor); ok {
			continue
		}
		pkgs++
		var members int
		for _, msg := range s.env.Messages {
			if _, ok := msg.(*messaging.SyntheticMessage); ok {
				members++
			}
		}
		assert.Equal(t, 1, members)
	}
	assert.Equal(t, 3, pkgs, "every message exceeds the budget — each goes alone")
}

// twoDestBurns builds one block of transfers split across BVN0 and BVN1, one
// per key, so the source's synthetic main chain grows by len(keys) entries in
// a single block. bvn0-bound/bvn1-bound route to the two partitions via the
// stub router.
func twoDestBurns(t *testing.T, keys [][]byte, ts uint64) *types.Batch {
	t.Helper()
	var txs [][]byte
	for i, key := range keys {
		from := protocol.LiteAuthorityForKey(key[32:], protocol.SignatureTypeED25519)
		dest := "bvn0-bound.acme"
		if i%2 == 1 {
			dest = "bvn1-bound.acme"
		}
		env, err := build.Transaction().For(from.JoinPath("ACME")).
			SendTokens(uint64(i+1), 0).To(protocol.AccountUrl(dest, from.Hostname())).
			SignWith(from).Version(1).Timestamp(ts).PrivateKey(key).
			Done()
		require.NoError(t, err)
		b, err := env.MarshalBinary()
		require.NoError(t, err)
		txs = append(txs, b)
	}
	return types.NewBatch(txs)
}

// #4150: a package's collection proof spans from its first member to the
// block's LAST synthetic element, so a block emitting more than
// MaxReceiptListElements synthetics builds spans past the receiver's hard
// bound. Before the clamp, the leader shipped those packages and every
// validator rejected them at CheckTx, stranding the block on healing — the
// exact high-volume case packaging exists for. Every dispatched envelope must
// now be acceptable: any package's ReceiptList within the bound and valid,
// and the leading over-span messages fall back to individual receipts.
func TestBridge_BlockOverMaxReceiptListElementsStaysAcceptable(t *testing.T) {
	if testing.Short() {
		t.Skip("drives >4096 transfers through one block")
	}

	// Comfortably past the 4096-element bound, split across two destinations.
	n := protocol.MaxReceiptListElements + 128
	keys := make([][]byte, n)
	for i := range keys {
		keys[i] = acctesting.GenerateKey("span", i)
	}
	r := newDnBridge(t, 2, 0, keys...)

	_, err := r.produce(t, twoDestBurns(t, keys, 1))
	require.NoError(t, err)
	sent := r.deliverOwnAnchorAndCollect(t)

	var members, packages, singles, overBound int
	for _, s := range sent {
		if _, ok := s.env.Messages[0].(*messaging.BlockAnchor); ok {
			continue // the DN's own anchor dispatch
		}
		if proof, ok := s.env.Messages[0].(*messaging.SyntheticProof); ok {
			packages++
			list := proof.Proof.ReceiptList
			require.NotNil(t, list)
			if len(list.Elements) > protocol.MaxReceiptListElements {
				overBound++
			}
			assert.True(t, list.Validate(nil), "every dispatched package's proof must be valid")
			for _, m := range s.env.Messages[1:] {
				syn, ok := m.(*messaging.SyntheticMessage)
				if !ok {
					continue
				}
				members++
				h := syn.Message.Hash()
				assert.True(t, list.Included(h[:]), "a member must be covered by its package's proof")
			}
			continue
		}
		// A fallback individual-receipt dispatch.
		syn, ok := s.env.Messages[0].(*messaging.SyntheticMessage)
		require.True(t, ok, "a non-anchor, non-package dispatch is an individual synthetic")
		require.NotNil(t, syn.Proof.Receipt, "the fallback carries its own receipt")
		require.Nil(t, syn.Proof.ReceiptList)
		assert.True(t, syn.Proof.Receipt.Validate(nil), "the fallback receipt must be valid")
		members++
		singles++
	}

	assert.Zero(t, overBound, "no dispatched package may exceed the receiver's element bound")
	assert.Positive(t, singles, "the over-span leaders must fall back to individual receipts")
	assert.Equal(t, n, members, "every synthetic is dispatched exactly once — nothing stranded")
}
