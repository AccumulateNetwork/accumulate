// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"crypto/ed25519"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// stagingSim isolates staging from the rest of the executor and lets a test
// drive it event by event: a package arrives, a Directory anchor executes, a
// block runs. The source is a real chain of sequenced messages so proofs cover
// real hashes; the sequenced layer is replaced by a fake that only moves the
// stream position, which is all staging observes of execution.
//
// The rules under test (executor spec, "Collection", "Proof", "Anchor
// staging"):
//   - a proof waits at the anchor stage until its Directory anchor executes,
//     which validates it or discards it;
//   - an entry whose hash a validated proof covers is proven and runs when
//     next; an unproven entry is collected and never run;
//   - once index i has executed, an entry at or below i is tossed; an entry
//     above the last validated index is held, within the horizon;
//   - a proof that contradicts what is already proven is tossed, and the
//     first stands.
type stagingSim struct {
	t      *testing.T
	x      *Executor
	batch  *database.Batch
	b      *Block
	c      *classified
	seqs   []*messaging.SequencedMessage
	chain  *database.Chain
	chain2 *database.Chain2
	root   *database.Chain
	roots  [][]byte // roots[k] is the root chain's anchor at height k
	str    stream
	block  uint64
	key    ed25519.PrivateKey // a validator of the source: a collected entry is held on its word
}

func newStagingSim(t *testing.T, entries int) *stagingSim {
	t.Helper()
	seed := sha256.Sum256([]byte("source validator"))
	key := ed25519.NewKeyFromSeed(seed[:])
	x := new(Executor)
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}
	x.globalsPtr.Store(&Globals{Active: core.GlobalValues{
		ExecutorVersion: protocol.ExecutorVersionLatest,
		Network: &protocol.NetworkDefinition{Version: 1, Validators: []*protocol.ValidatorInfo{{
			PublicKey:     key[32:],
			PublicKeyHash: sha256.Sum256(key[32:]),
			Partitions:    []*protocol.ValidatorPartitionInfo{{ID: "BVN1", Active: true}},
		}}},
	}})
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	s := &stagingSim{t: t, x: x, batch: batch, key: key}
	s.str = stream{kind: streamSynthetic, ledger: x.Describe.Synthetic(), source: protocol.PartitionUrl("BVN1")}

	// The sequenced layer, reduced to what staging sees of it: next executes
	// and advances the stream, not-next is held.
	x.messageExecutors = map[messaging.MessageType]ExecutorFactory2[messaging.MessageType, *MessageContext]{
		messaging.MessageTypeSequenced: func(*MessageContext) (ExecutorFor[messaging.MessageType, *MessageContext], bool) {
			return simSequencedExecutor{s}, true
		},
	}

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = x.Describe.Synthetic()
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))
	pool := new(protocol.AnchorLedger)
	pool.Url = x.Describe.AnchorPool()
	require.NoError(t, batch.Account(pool.Url).Main().Put(pool))

	// The source's synthetic chain: real sequenced messages, numbered 1..n.
	s.chain2 = batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Synthetic)).MainChain()
	c, err := s.chain2.Get()
	require.NoError(t, err)
	s.chain = c
	// The source's root chain anchors the synthetic chain once per entry, as
	// a block would at its end; the Directory root in this simulation is the
	// root chain's final anchor.
	root, err := batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(t, err)
	for i := 0; i < entries; i++ {
		txn := new(protocol.Transaction)
		txn.Header.Principal = protocol.AccountUrl("alice", "tokens")
		txn.Body = &protocol.SyntheticDepositCredits{Amount: uint64(i + 1)}
		seq := &messaging.SequencedMessage{
			Message:     &messaging.TransactionMessage{Transaction: txn},
			Source:      protocol.PartitionUrl("BVN1"),
			Destination: protocol.PartitionUrl("BVN0"),
			Number:      uint64(i + 1),
		}
		h := seq.Hash()
		require.NoError(t, c.AddEntry(h[:], false))
		require.NoError(t, root.AddEntry(c.Anchor(), false))
		s.roots = append(s.roots, append([]byte(nil), root.Anchor()...))
		s.seqs = append(s.seqs, seq)
	}
	s.root = root

	s.newBlock()
	return s
}

// newBlock starts a fresh block: new positions, new classification, nothing
// carried over but the database — which is exactly what a real block sees.
func (s *stagingSim) newBlock() {
	if s.b != nil {
		// Close the previous block: Delivered is written back to the ledger.
		require.NoError(s.t, s.b.flushStreams())
		s.b.staging.Commit()
	}
	s.block++
	s.b = &Block{positions: new(positionCache), Executor: s.x, Batch: s.batch, staging: s.x.staging().Begin()}
	// Staging carries the index of the block that publishes it, as Begin sets
	// it on a real block (#4291): a snapshot taken here says which block it is
	// as of.
	s.b.staging.AtBlock(s.block)
	s.c = &classified{streams: map[string]stream{}, arrivals: map[string]map[uint64]*arrival{}}
	s.c.addStream(s.str)
}

// rootAt is the Directory root that anchor block k carries in this
// simulation: the source's root chain anchored at height k. Distinct per k,
// so a proof under block k is admissible only once anchor k has executed.
func (s *stagingSim) rootAt(k uint64) []byte {
	s.t.Helper()
	require.LessOrEqual(s.t, k, uint64(len(s.roots)))
	return s.roots[k-1]
}

// proof builds the package proof a source would send for entries [first,
// last] under Directory anchor block anchorBlock (which must cover last).
func (s *stagingSim) proof(first, last int, anchorBlock uint64) *protocol.AnnotatedReceipt {
	s.t.Helper()
	require.Greater(s.t, anchorBlock, uint64(last), "an anchor covers the entries before it")
	list, err := merkle.GetReceiptList(s.chain2.Inner(), int64(first), int64(last))
	require.NoError(s.t, err)
	// Continue from the root chain entry that anchors the chain as of `last`
	// to the root chain as anchor block anchorBlock saw it.
	cont, err := s.root.Receipt(int64(last), int64(anchorBlock)-1)
	require.NoError(s.t, err)
	list.ContinuedReceipt = cont
	require.True(s.t, list.Validate(nil), "the simulation's proof must be valid")
	return &protocol.AnnotatedReceipt{ReceiptList: list, Anchor: directoryAnchorMetadata(anchorBlock)}
}

func (s *stagingSim) member(i int) *messaging.SyntheticMessage {
	return &messaging.SyntheticMessage{Message: s.seqs[i], Signature: s.sign(s.seqs[i])}
}

// sign is the source validator's signature over a sequenced message, what a
// dispatched copy carries.
func (s *stagingSim) sign(seq *messaging.SequencedMessage) protocol.KeySignature {
	h := seq.Hash()
	sig := &protocol.ED25519Signature{PublicKey: s.key[32:], Signer: s.str.source.JoinPath(protocol.Network), SignerVersion: 1, TransactionHash: h}
	protocol.SignED25519(sig, s.key, nil, h[:])
	return sig
}

// packageArrives is a package envelope reaching this block: the proof goes
// through intake, then each member is processed as the block would process
// the envelope. Returns the members' status codes.
func (s *stagingSim) packageArrives(first, last int, anchorBlock uint64) []errors.Status {
	s.t.Helper()
	proof := s.proof(first, last, anchorBlock)
	env := []messaging.Message{&messaging.SyntheticProof{Proof: proof}}
	var siblings [][]byte
	for i := first; i <= last; i++ {
		env = append(env, s.member(i))
		h := s.seqs[i].Hash()
		siblings = append(siblings, h[:])
	}
	err := s.b.intakeProof(s.str.source, proof, siblings)
	if err != nil && !errors.Is(err, errors.BadRequest) {
		require.NoError(s.t, err)
	}
	var codes []errors.Status
	for i := first; i <= last; i++ {
		codes = append(codes, s.process(env, env[1+i-first]))
	}
	return codes
}

// packageEnvelope is the envelope a package travels in — the proof and the
// members it covers — as a block's batches carry it. It is what
// packageArrives feeds the executing node, for a test that feeds the same
// block to a node that only collects it (#4292).
func (s *stagingSim) packageEnvelope(first, last int, anchorBlock uint64) *messaging.Envelope {
	s.t.Helper()
	msgs := []messaging.Message{&messaging.SyntheticProof{Proof: s.proof(first, last, anchorBlock)}}
	for i := first; i <= last; i++ {
		msgs = append(msgs, s.member(i))
	}
	return &messaging.Envelope{Messages: msgs}
}

// process runs one message of an envelope through its executor, the way the
// block (and MessageIsReady, for a staged one) dispatches by message type.
func (s *stagingSim) process(env []messaging.Message, msg messaging.Message) errors.Status {
	s.t.Helper()
	d := &bundle{Block: s.b, batch: s.batch, messages: env}
	ctx := &MessageContext{bundle: d, message: msg}
	var st *protocol.TransactionStatus
	var err error
	switch msg.(type) {
	case *messaging.SequencedMessage:
		st, err = simSequencedExecutor{s}.Process(s.batch, ctx)
	default:
		st, err = SyntheticMessage{}.Process(s.batch, ctx)
	}
	require.NoError(s.t, err)
	if st == nil {
		return 0
	}
	return st.Code
}

// anchorExecutes is a Directory anchor for block n carrying root landing on
// this node: its root goes on the Directory anchor chain, the newest anchor
// block advances, and the block records it so staged proofs are decided.
func (s *stagingSim) anchorExecutes(n uint64, root []byte) {
	s.t.Helper()
	c, err := s.batch.Account(s.x.Describe.AnchorPool()).AnchorChain(protocol.Directory).Root().Get()
	require.NoError(s.t, err)
	require.NoError(s.t, c.AddEntry(root, false))
	require.NoError(s.t, s.batch.Account(s.x.Describe.AnchorPool()).DirectoryAnchorBlock().Put(n))
	body := &protocol.DirectoryAnchor{}
	body.MinorBlockIndex = n
	copy(body.RootChainAnchor[:], root)
	s.b.State.ReceivedAnchors = append(s.b.State.ReceivedAnchors, &chain.ReceivedAnchor{Partition: protocol.Directory, Body: body})
	require.NoError(s.t, s.b.validateStagedProofs(s.c))
}

// run is the synthetic group of a block: the stream's run is built from what
// is held and proven, and each entry is executed the way MessageIsReady does.
func (s *stagingSim) run() (executed []uint64) {
	s.t.Helper()
	pos, err := s.b.positionOf(s.str)
	require.NoError(s.t, err)
	run, _ := buildRun(pos, nil, 1024)
	for _, e := range run {
		require.NotNil(s.t, e.staged, "the simulation feeds arrivals directly; runs hold staged ids")
		held, ok := s.b.staging.HeldByID(e.staged)
		require.True(s.t, ok, "a staged id is held in staging")
		loaded := held.Message
		before := pos.delivered
		s.process([]messaging.Message{loaded}, loaded)
		if pos.delivered > before {
			executed = append(executed, e.number)
		}
	}
	return executed
}

func (s *stagingSim) delivered() uint64 {
	pos, err := s.b.positionOf(s.str)
	require.NoError(s.t, err)
	return pos.delivered
}

func (s *stagingSim) held(n uint64) bool {
	_, ok := s.b.staging.IDOf(s.str.id(), n)
	return ok
}

func (s *stagingSim) collected(n uint64) bool {
	h, ok := s.b.staging.IDOf(s.str.id(), n)
	return ok && h.Collected
}

func (s *stagingSim) proven(i int) bool {
	return s.b.staging.IsValidated(s.str.id(), s.seqs[i].Number, s.seqs[i].Hash())
}

// simSequencedExecutor stands in for the sequenced layer.
type simSequencedExecutor struct{ s *stagingSim }

func (e simSequencedExecutor) Validate(*database.Batch, *MessageContext) (*protocol.TransactionStatus, error) {
	return nil, nil
}

func (e simSequencedExecutor) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	seq := ctx.message.(*messaging.SequencedMessage)
	pos, err := ctx.Block.positionOf(e.s.str)
	if err != nil {
		return nil, err
	}
	st := &protocol.TransactionStatus{TxID: seq.ID()}
	switch {
	case seq.Number <= pos.delivered:
		st.Code = errors.Delivered
	case seq.Number == pos.next():
		err = ctx.Block.advanceStream(e.s.str, true, seq.Number, seq.ID(), seq)
		st.Code = errors.Delivered
	default:
		// Not next: the sequenced layer holds it (it passed its proof).
		err = ctx.Block.advanceStream(e.s.str, false, seq.Number, seq.ID(), seq)
		st.Code = errors.Pending
	}
	return st, err
}

// ---------------------------------------------------------------------------

func TestStaging_PackageAheadOfItsAnchor_IsCollectedThenRuns(t *testing.T) {
	s := newStagingSim(t, 6)

	codes := s.packageArrives(0, 2, 3)
	for _, c := range codes {
		require.Equal(t, errors.Pending, c, "collected, not executed")
	}
	for n := uint64(1); n <= 3; n++ {
		require.True(t, s.held(n), "held at its number")
		require.True(t, s.collected(n), "marked collected")
		require.False(t, s.proven(int(n-1)))
	}
	require.Equal(t, uint64(0), s.delivered())
	require.Empty(t, s.run(), "a collected entry is never run")

	// The anchor lands: the proof validates, the entries are proven and run.
	s.newBlock()
	s.anchorExecutes(3, s.rootAt(3))
	for n := 0; n < 3; n++ {
		require.True(t, s.proven(n))
	}
	require.Equal(t, []uint64{1, 2, 3}, s.run())
	require.Equal(t, uint64(3), s.delivered())
}

func TestStaging_PackageAfterItsAnchor_ExecutesAtOnce(t *testing.T) {
	s := newStagingSim(t, 6)
	s.anchorExecutes(3, s.rootAt(3))
	codes := s.packageArrives(0, 2, 3)
	for _, c := range codes {
		require.Equal(t, errors.Delivered, c)
	}
	require.Equal(t, uint64(3), s.delivered())
	for n := uint64(1); n <= 3; n++ {
		require.False(t, s.collected(n), "nothing collected: it executed")
	}
}

func TestStaging_EntriesAtOrBelowDeliveredAreTossed(t *testing.T) {
	s := newStagingSim(t, 6)
	s.anchorExecutes(3, s.rootAt(3))
	s.packageArrives(0, 2, 3)
	require.Equal(t, uint64(3), s.delivered())

	// A copy of 1..3 arrives again, and 2 arrives alone without a proof.
	s.newBlock()
	s.packageArrives(0, 2, 3)
	code := s.process([]messaging.Message{s.member(1)}, s.member(1))
	require.NotEqual(t, errors.Pending, code)
	require.Equal(t, uint64(3), s.delivered(), "nothing moved")
	for n := uint64(1); n <= 3; n++ {
		require.False(t, s.collected(n), "an entry at or below the delivered point is tossed, not collected")
	}
}

// A copy of an already-delivered entry arriving BEFORE the anchor it names is
// tossed silently: it must not fail its envelope, whose other members may be
// new. This was the leg that reopened holes in run 20260904T163... — the
// healer's copies and re-dispatched packages all took it.
func TestStaging_DeliveredEntryAheadOfItsAnchorIsTossedSilently(t *testing.T) {
	s := newStagingSim(t, 6)
	s.anchorExecutes(3, s.rootAt(3))
	s.packageArrives(0, 2, 3)
	require.Equal(t, uint64(3), s.delivered())

	// A package for 1..4 under anchor 4, which has not executed: 1..3 are
	// delivered already, 4 is new. Nothing may fail; 4 is collected.
	s.newBlock()
	codes := s.packageArrives(0, 3, 4)
	require.Len(t, codes, 4)
	require.Equal(t, errors.Pending, codes[3], "the new entry is collected")
	require.True(t, s.held(4))
	require.True(t, s.collected(4))
	for n := uint64(1); n <= 3; n++ {
		require.False(t, s.collected(n), "delivered entries are tossed, not collected")
	}
	s.anchorExecutes(4, s.rootAt(4))
	require.Equal(t, []uint64{4}, s.run())
}

func TestStaging_EntriesAboveTheLastValidatedAreHeld_WithinTheHorizon(t *testing.T) {
	s := newStagingSim(t, 6)
	// Entries 4..6 arrive proven, out of order, before 1..3 exist here: held
	// by the sequenced layer, not run.
	s.anchorExecutes(6, s.rootAt(6))
	s.packageArrives(3, 5, 6)
	for n := uint64(4); n <= 6; n++ {
		require.True(t, s.held(n))
		require.False(t, s.collected(n), "proven when held: no collected mark")
	}
	require.Empty(t, s.run())

	// 1..3 arrive; everything drains in order.
	s.newBlock()
	s.packageArrives(0, 2, 6)
	require.Equal(t, uint64(3), s.delivered())
	require.Equal(t, []uint64{4, 5, 6}, s.run())
	require.Equal(t, uint64(6), s.delivered())

	// Beyond the horizon is refused, not held.
	farSeq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: s.seqs[0].Message.(*messaging.TransactionMessage).Transaction},
		Source:      s.str.source,
		Destination: protocol.PartitionUrl("BVN0"),
		Number:      6 + maxSequenceAhead + 1,
	}
	farHash := farSeq.Hash()
	far := &messaging.SyntheticMessage{
		Message:   farSeq,
		Proof:     &protocol.AnnotatedReceipt{Anchor: directoryAnchorMetadata(99), Receipt: &merkle.Receipt{Start: farHash[:], Anchor: farHash[:]}},
		Signature: s.sign(farSeq), // a validator's word, so the horizon is what refuses it
	}
	d := &bundle{Block: s.b, batch: s.batch, messages: []messaging.Message{far}}
	_, err := SyntheticMessage{}.Process(s.batch, &MessageContext{bundle: d, message: far})
	require.NoError(t, err)
	require.False(t, s.held(6+maxSequenceAhead+1), "refused, not held")
}

func TestStaging_DisprovedProofLeavesEntriesWaitingForARealOne(t *testing.T) {
	s := newStagingSim(t, 8)
	s.packageArrives(0, 2, 6)
	other := make([]byte, 32)
	other[0] = 0xEE
	disproved0 := count("disproved")
	s.anchorExecutes(6, other) // anchor 6 does not carry the claimed root
	require.Equal(t, disproved0+1, count("disproved"))
	for n := uint64(1); n <= 3; n++ {
		require.True(t, s.held(n), "the entries stay, waiting")
		require.False(t, s.proven(int(n-1)))
	}
	require.Empty(t, s.run())

	// The source re-dispatches under a later anchor, which does carry it.
	s.newBlock()
	s.packageArrives(0, 2, 8)
	s.anchorExecutes(8, s.rootAt(8))
	require.Equal(t, []uint64{1, 2, 3}, s.run())
}

func TestStaging_ConflictingProofIsTossed_TheFirstStands(t *testing.T) {
	s := newStagingSim(t, 3)
	s.anchorExecutes(3, s.rootAt(3))
	s.packageArrives(0, 2, 3)
	require.Equal(t, uint64(3), s.delivered())

	// A well-formed proof from another chain claiming the same indexes.
	otherChain := s.batch.Account(protocol.PartitionUrl("BVN2").JoinPath(protocol.Synthetic)).MainChain()
	c, err := otherChain.Get()
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		h := make([]byte, 32)
		h[0], h[1] = 0xF0, byte(i)
		require.NoError(t, c.AddEntry(h, false))
	}
	forged, err := merkle.GetReceiptList(otherChain.Inner(), 0, 2)
	require.NoError(t, err)
	conflict0 := count("conflict")
	b := s.b
	require.NoError(t, b.proofValidated(s.str.source, &protocol.AnnotatedReceipt{ReceiptList: forged, Anchor: directoryAnchorMetadata(3)}))
	require.Equal(t, conflict0+1, count("conflict"))
	for i := 0; i < 3; i++ {
		require.True(t, s.proven(i), "the first proof stands")
	}
	require.False(t, s.b.staging.IsValidated(s.str.id(), 2, to32(forged.Elements[1])), "the forged one proves nothing")
}

func to32(b []byte) [32]byte {
	var h [32]byte
	copy(h[:], b)
	return h
}
