// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4145: execution is sharded by IDENTITY — u.Routing(), the same number
// that assigns an account to its partition, taken one level finer. A shard
// is a sub-partition. The property to preserve: shard count cannot change
// the result. It holds because each shard executes its own envelopes in
// arrival order, no transaction writes outside its identity (#4143, asserted
// and empirically confirmed), and sequencing is deferred to the sorted
// block-end pass (#4144).
//
// The rules, one per cross-shard hazard:
//   (i)   system, anchor, synthetic and sequenced messages execute serially,
//         outside the parallel phase; system accounts are read-only within it
//   (ii)  an envelope whose messages span identities executes serially
//   (iii) staged deliveries run only in the serial lane, because
//         every synthetic delivery is serial — a later phase moves
//         a stage may span identities and must not widen a bundle's
//   (iv)  each shard executes against its own child batch; every touch of
//         the shared parent is serialized through one mutex, and commits
//         happen serially in shard order
//   (v)   bundle results merge into block state serially: shard order, then
//         arrival order within a shard
//
// A serial envelope is a BARRIER: consecutive shardable envelopes form a
// run that executes in parallel; the barrier executes alone, in arrival
// order, exactly where it stood. At every shard count the observable result
// is identical to serial arrival-order execution.

// envelopeIdentity returns the single identity the envelope's messages
// belong to, or (nil, false) if the envelope must execute serially: any
// non-user message, any signature that is not a user key signature, any
// system or partition identity, any transaction that cannot be resolved to
// its real content, any held (HoldUntil) transaction, or messages spanning
// more than one identity.
//
// Classification must never trust a submitter's CLAIM about a transaction.
// A remote stub's principal and a signature's TxID account are claims: the
// executor loads the real transaction by HASH and writes ITS principal's
// records, so classifying by the claim would let a crafted envelope execute
// another identity's writes on the wrong shard (#4149). Claims are resolved
// to the real transaction — from this envelope or from the store — and
// anything unresolvable is serial.
func (b *Block) envelopeIdentity(batch *database.Batch, messages []messaging.Message) (*url.URL, bool) {
	var identity *url.URL
	add := func(u *url.URL) bool {
		// Network accounts (dn.acme/*, bvn-*.acme/*) are user-writable but
		// mutate shared executor state (pending globals), and ACME is
		// written by system production — all serial. Applied to every
		// identity, signers included: an operator page signature writes
		// partition accounts.
		if _, isPart := protocol.ParsePartitionUrl(u); isPart ||
			protocol.AcmeUrl().LocalTo(u) {
			return false
		}
		id := u.RootIdentity()
		switch {
		case identity == nil:
			identity = id
			return true
		case identity.Equal(id):
			return true
		default:
			return false // multi-identity → serial (hazard ii)
		}
	}

	// Full transactions travelling in this envelope, by hash. A full
	// transaction cannot lie about its principal — the hash covers the
	// header — so it is an authoritative source when a signature or a
	// remote stub refers to it.
	var local map[[32]byte]*protocol.Transaction
	for _, msg := range messages {
		txm, ok := msg.(*messaging.TransactionMessage)
		if !ok || txm.Transaction.Body.Type() == protocol.TransactionTypeRemote {
			continue
		}
		if local == nil {
			local = map[[32]byte]*protocol.Transaction{}
		}
		local[txm.Transaction.ID().Hash()] = txm.Transaction
	}

	// resolve returns the REAL transaction for a hash, or nil (→ serial).
	resolve := func(hash [32]byte) *protocol.Transaction {
		if txn, ok := local[hash]; ok {
			return txn
		}
		msg, err := batch.Message(hash).Main().Get()
		if err != nil {
			return nil
		}
		txm, ok := msg.(*messaging.TransactionMessage)
		if !ok || txm.Transaction.Body.Type() == protocol.TransactionTypeRemote {
			return nil
		}
		return txm.Transaction
	}

	addTxn := func(txn *protocol.Transaction) bool {
		if !txn.Body.Type().IsUser() {
			return false // hazard (i)
		}
		// A held transaction's signatures write the partition ledger's
		// event records — a system account — from whichever shard the
		// envelope lands on (#4149). Serial until that write moves into
		// block state.
		if txn.Header.HoldUntil != nil {
			return false
		}
		if txn.Header.Principal == nil {
			return false
		}
		return add(txn.Header.Principal)
	}

	for _, msg := range messages {
		switch msg := msg.(type) {
		case *messaging.TransactionMessage:
			txn := msg.Transaction
			if txn.Body.Type() == protocol.TransactionTypeRemote {
				// Processing a remote stub means loading — and possibly
				// executing — the real transaction. Classify by that.
				txn = resolve(msg.Hash())
				if txn == nil {
					return nil, false
				}
			}
			if !addTxn(txn) {
				return nil, false
			}

		case *messaging.SignatureMessage:
			sig, ok := msg.Signature.(protocol.UserSignature)
			if !ok {
				return nil, false // system signatures are serial
			}
			signer := sig.RoutingLocation()
			if signer == nil || msg.TxID == nil {
				return nil, false
			}
			// The signature writes the signer's identity and executes
			// against the transaction with this HASH. The common self-signed
			// case is one identity and stays parallel; cross-identity
			// signing (delegation, multisig across ADIs) goes serial
			// (hazard ii).
			if !add(signer) {
				return nil, false
			}
			txn := resolve(msg.TxID.Hash())
			if txn == nil || !addTxn(txn) {
				return nil, false
			}

		default:
			return nil, false // anchors, synthetics, sequenced, internal — serial
		}
	}

	return identity, identity != nil
}

// ProcessAll processes a set of envelopes, executing runs of consecutive
// single-identity user envelopes across ExecutionShards shards. With shard
// count <= 1 it is exactly a loop over Process. Each envelope's outcome is
// reported independently, so callers keep their own error policy.
func (b *Block) ProcessAll(envelopes []*messaging.Envelope) []*execute.ProcessResult {
	results := make([]*execute.ProcessResult, len(envelopes))

	// Staging decides the block's order before anything runs (#4169 step 6).
	//
	//   1. Anchor streams, first, because an anchor extends the directory root
	//      that admits synthetics — so a synthetic can use an anchor that
	//      arrived in this same block instead of waiting one.
	//   2. Synthetic streams, decided against the chain the anchors just
	//      extended.
	//   3. The envelope loop below: user envelopes, and anything staging did
	//      not place in a run. Last, so a deposit lands before a transaction
	//      spends.
	//   4. Drain again, because step 3 REVEALS WORK TO THE BLOCK. An envelope
	//      that arrives out of sequence is recorded pending, and that makes it
	//      drainable now, not next block.
	//
	// Sequenced messages never reached the sharded path anyway —
	// envelopeIdentity classified them serial — so moving them here takes work
	// out of the loop below rather than changing where it runs.
	c := b.classify(envelopes)
	ran := map[int]bool{}

	fail := func(err error) []*execute.ProcessResult {
		for i := range results {
			results[i] = &execute.ProcessResult{Error: err, Shard: -1}
		}
		return results
	}

	// drain decides and runs every stream once, in group order. Each round
	// decides against the positions as the previous round left them — the
	// position is the block's live state of the stream (#4169 step 7), so
	// nothing needs re-reading.
	drain := func() (int, error) {
		n := 0
		for _, kind := range []streamKind{streamAnchor, streamSynthetic} {
			runs, err := b.stageRuns(c, kind)
			if err != nil {
				return n, err
			}
			n += b.executeRuns(runs, results, ran)
		}
		return n, nil
	}

	if _, err := drain(); err != nil {
		return fail(err)
	}
	staged := ran

	shards := b.Executor.ExecutionShards
	if shards <= 1 {
		// The serial path, verbatim.
		for i, env := range envelopes {
			if staged[i] {
				continue // already executed, in staging's order
			}
			s, err := b.Process(env)
			results[i] = &execute.ProcessResult{Statuses: s, Error: err, Shard: -1}
		}
		b.drainRevealed(drain)
		return results
	}

	type item struct {
		index    int
		messages []messaging.Message
	}
	pending := make([][]item, shards)
	var pendingCount int

	// flush executes the accumulated run: shards in parallel, then commits
	// and merges serially in (shard, arrival) order.
	flush := func() {
		if pendingCount == 0 {
			return
		}
		pendingCount = 0

		type shardOut struct {
			batch   *database.Batch
			bundles [][]*bundle // parallel to pending[i]
		}
		out := make([]shardOut, shards)

		// One mutex serializes every touch of the shared parent batch
		// (hazard iv) — each shard's child memoizes what it pulls through,
		// so the lock is paid once per record per shard. The children are
		// created HERE, before the goroutines: BeginConcurrent mutates the
		// parent (the child id counter) outside the mutex, so calling it
		// from the shards is a data race (#4149).
		var mu sync.Mutex
		for s := range pending {
			if len(pending[s]) == 0 {
				continue
			}
			out[s].batch = b.Batch.BeginConcurrent(&mu, true)
		}

		var wg sync.WaitGroup
		for s := range pending {
			if out[s].batch == nil {
				continue
			}
			wg.Add(1)
			go func(s int) {
				defer wg.Done()
				out[s].bundles = make([][]*bundle, len(pending[s]))
				for i, it := range pending[s] {
					statuses, bundles, err := b.processEnvelope(out[s].batch, it.messages)
					results[it.index] = &execute.ProcessResult{Statuses: statuses, Error: err, Shard: s}
					out[s].bundles[i] = bundles
				}
			}(s)
		}
		wg.Wait()

		// Serial phase: commit child batches and merge bundle results in
		// fixed shard order (hazard v).
		for s := range pending {
			if out[s].batch == nil {
				continue
			}
			if b.fatal != nil {
				// The parent batch may already hold a partial commit —
				// nothing more may be committed on top of it.
				out[s].batch.Discard()
				for _, it := range pending[s] {
					results[it.index] = &execute.ProcessResult{Error: b.fatal, Shard: -1}
				}
				continue
			}
			err := out[s].batch.Commit()
			if err != nil {
				// Commit writes value by value and stops at the first
				// error, so a failure here may have already written a
				// PREFIX of the child's state into the parent. The block
				// is corrupt: poison it so Close refuses to produce a
				// state hash from it (#4149). Degrading per-shard would
				// silently commit the corruption.
				b.fatal = errors.FatalError.WithFormat("commit shard %d: %w", s, err)
				out[s].batch.Discard()
				for _, it := range pending[s] {
					results[it.index] = &execute.ProcessResult{Error: b.fatal, Shard: -1}
				}
				continue
			}
			for _, bundles := range out[s].bundles {
				for _, d := range bundles {
					d.mergeIntoBlock()
				}
			}
		}
		for s := range pending {
			pending[s] = pending[s][:0]
		}
	}

	for i, env := range envelopes {
		if b.fatal != nil {
			// A shard commit failure poisoned the block — stop executing.
			results[i] = &execute.ProcessResult{Error: b.fatal, Shard: -1}
			continue
		}

		if staged[i] {
			continue // already executed, in staging's order
		}

		messages, err := env.Normalize()
		if err != nil {
			// A malformed envelope is its own outcome; it does not interrupt
			// the run.
			results[i] = &execute.ProcessResult{Error: errors.UnknownError.Wrap(err), Shard: -1}
			continue
		}

		identity, ok := b.envelopeIdentity(b.Batch, messages)
		if !ok {
			// Barrier: finish the parallel run, then execute serially in
			// place (hazards i and ii).
			flush()
			if b.fatal != nil {
				results[i] = &execute.ProcessResult{Error: b.fatal, Shard: -1}
				continue
			}
			statuses, bundles, err := b.processEnvelope(b.Batch, messages)
			if err == nil {
				for _, d := range bundles {
					d.mergeIntoBlock()
				}
			}
			results[i] = &execute.ProcessResult{Statuses: statuses, Error: err, Shard: -1}
			continue
		}

		s := int(identity.Routing() % uint64(shards))
		pending[s] = append(pending[s], item{index: i, messages: messages})
		pendingCount++
	}
	flush()
	b.drainRevealed(drain)

	return results
}
