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
//   (iii) the MessageIsReady cascade runs only in the serial lane, because
//         every synthetic delivery is serial — a later phase moves
//         cross-identity cascades onto the next-block queue (#4146)
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
// system or partition identity, or messages spanning more than one identity.
func (b *Block) envelopeIdentity(messages []messaging.Message) (*url.URL, bool) {
	var identity *url.URL
	add := func(u *url.URL) bool {
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

	for _, msg := range messages {
		switch msg := msg.(type) {
		case *messaging.TransactionMessage:
			if !msg.Transaction.Body.Type().IsUser() {
				return nil, false // hazard (i)
			}
			principal := msg.Transaction.Header.Principal
			if principal == nil {
				return nil, false
			}
			// Network accounts (dn.acme/*, bvn-*.acme/*) are user-writable
			// but mutate shared executor state (pending globals), and ACME is
			// written by system production — all serial.
			if _, isPart := protocol.ParsePartitionUrl(principal); isPart ||
				protocol.AcmeUrl().LocalTo(principal) {
				return nil, false
			}
			if !add(principal) {
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
			// The signature writes the signer's identity and refers to a
			// transaction on its principal's identity. The common self-signed
			// case is one identity and stays parallel; cross-identity signing
			// (delegation, multisig across ADIs) goes serial (hazard ii).
			if !add(signer) || !add(msg.TxID.Account()) {
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

	shards := b.Executor.ExecutionShards
	if shards <= 1 {
		// The serial path, verbatim.
		for i, env := range envelopes {
			s, err := b.Process(env)
			results[i] = &execute.ProcessResult{Statuses: s, Error: err}
		}
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
		// so the lock is paid once per record per shard.
		var mu sync.Mutex
		var wg sync.WaitGroup
		for s := range pending {
			if len(pending[s]) == 0 {
				continue
			}
			wg.Add(1)
			go func(s int) {
				defer wg.Done()
				batch := b.Batch.BeginConcurrent(&mu, true)
				out[s].batch = batch
				out[s].bundles = make([][]*bundle, len(pending[s]))
				for i, it := range pending[s] {
					statuses, bundles, err := b.processEnvelope(batch, it.messages)
					results[it.index] = &execute.ProcessResult{Statuses: statuses, Error: err}
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
			err := out[s].batch.Commit()
			if err != nil {
				// A commit failure would corrupt the block — report it on
				// every envelope of the shard rather than committing a
				// partial run.
				for _, it := range pending[s] {
					results[it.index] = &execute.ProcessResult{Error: errors.UnknownError.WithFormat("commit shard %d: %w", s, err)}
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
		messages, err := env.Normalize()
		if err != nil {
			// A malformed envelope is its own outcome; it does not interrupt
			// the run.
			results[i] = &execute.ProcessResult{Error: errors.UnknownError.Wrap(err)}
			continue
		}

		identity, ok := b.envelopeIdentity(messages)
		if !ok {
			// Barrier: finish the parallel run, then execute serially in
			// place (hazards i and ii).
			flush()
			statuses, bundles, err := b.processEnvelope(b.Batch, messages)
			if err == nil {
				for _, d := range bundles {
					d.mergeIntoBlock()
				}
			}
			results[i] = &execute.ProcessResult{Statuses: statuses, Error: err}
			continue
		}

		s := int(identity.Routing() % uint64(shards))
		pending[s] = append(pending[s], item{index: i, messages: messages})
		pendingCount++
	}
	flush()

	return results
}
