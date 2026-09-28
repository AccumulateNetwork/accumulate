// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TakeHistory takes, before the node executes the block after v, what its
// peers know of the transactions it is about to execute and it does not: that
// one of them was delivered already. A transaction is skipped when its status
// says it was delivered, and the status is not in the BPT, so a pull does not
// bring it. A transaction that is not unique -- a burn of ACME is a
// SyntheticBurnTokens carrying only its amount, so two burns of one amount are
// one transaction -- is skipped by the peers and executed by a node that
// joined: every Directory follower of run 20260928T051402Z burned again on the
// first block after its proof (#4442).
//
// The transactions asked about are the ones queued as local deliveries in the
// state at v, drained by the next block, and the ones inside what staging
// holds. A transaction counts as delivered by v when a peer's status says it
// was delivered and first received at or before v, or, where the peer serves
// no status (a store that no longer reads it), when its hash is on its
// principal's main chain below the height this node's state at v gives that
// chain.
func (s *PulledState) TakeHistory(ctx context.Context, v uint64, staged []*url.TxID) {
	ids := append(s.queuedLocalDeliveries(), staged...)
	if len(ids) == 0 {
		return
	}
	srcs := s.partitionSources(ctx)
	var byStatus, byChain, notDelivered, held, failed int
	seen := map[[32]byte]bool{}
	for _, id := range ids {
		h := id.Hash()
		if seen[h] || ctx.Err() != nil {
			continue
		}
		seen[h] = true
		if s.statusDelivered(h) {
			held++
			continue
		}
		how, err := s.deliveredBy(ctx, srcs, id, v)
		switch {
		case err != nil:
			failed++
			s.log.Info("Whether a transaction the next block executes was delivered already could not be learned",
				"partition", s.partition, "txid", id, "error", err)
			continue
		case how == "":
			notDelivered++
			continue
		case how == "status":
			byStatus++
		default:
			byChain++
		}
		if err := s.putDelivered(id, v); err != nil {
			failed++
			s.log.Info("A delivered transaction's status could not be written", "partition", s.partition, "txid", id, "error", err)
			continue
		}
		s.log.Info("A transaction the next block would execute was delivered already by its peers; taking its status",
			"partition", s.partition, "block", v, "txid", id, "by", how)
	}
	s.log.Info("Took the history of the transactions the next block executes", "partition", s.partition, "block", v,
		"asked", len(seen), "held", held, "by-status", byStatus, "by-chain", byChain, "not-delivered", notDelivered, "failed", failed)
}

// queuedLocalDeliveries is the local delivery queue of the synthetic ledger
// this node holds.
func (s *PulledState) queuedLocalDeliveries() []*url.TxID {
	batch := s.db.Begin(false)
	defer batch.Discard()
	ids, err := batch.Account(s.partition.JoinPath(protocol.Synthetic)).LocalDeliveryQueue().Get()
	if err != nil {
		return nil
	}
	return ids
}

func (s *PulledState) statusDelivered(h [32]byte) bool {
	batch := s.db.Begin(false)
	defer batch.Discard()
	st, err := batch.Transaction(h[:]).Status().Get()
	return err == nil && st.Delivered()
}

// deliveredBy says how a peer showed the transaction delivered by v: "status",
// "chain", or "" when no peer did.
func (s *PulledState) deliveredBy(ctx context.Context, srcs []pull.Source, id *url.TxID, v uint64) (string, error) {
	h := id.Hash()
	var last error
	answered := false
	for _, src := range srcs {
		rec, err := src.QueryMessage(ctx, id, nil)
		switch {
		case err == nil && rec != nil:
			answered = true
			st := &protocol.TransactionStatus{Code: rec.Status}
			if st.Delivered() && rec.Received != 0 && rec.Received <= v {
				return "status", nil
			}
		case errors.Is(err, errors.NotFound):
			answered = true
		default:
			last = err
		}

		// The principal's main chain: an executed transaction is an entry
		// of it. Below the height this node's state at v gives the chain,
		// the entry was there at v.
		height := s.mainChainHeight(id.Account())
		if height == 0 {
			continue
		}
		e, err := src.QueryChainEntry(ctx, id.Account(), &api.ChainQuery{Name: "main", Entry: h[:]})
		switch {
		case err == nil && e != nil:
			answered = true
			if e.Index < height {
				return "chain", nil
			}
		case errors.Is(err, errors.NotFound):
			answered = true
		default:
			last = err
		}
		if answered {
			return "", nil
		}
	}
	if answered {
		return "", nil
	}
	if last == nil {
		last = errors.NotFound.With("no peer to ask")
	}
	return "", last
}

func (s *PulledState) mainChainHeight(u *url.URL) uint64 {
	batch := s.db.Begin(false)
	defer batch.Discard()
	head, err := batch.Account(u).MainChain().Head().Get()
	if err != nil {
		return 0
	}
	return uint64(head.Count)
}

func (s *PulledState) putDelivered(id *url.TxID, v uint64) error {
	batch := s.db.Begin(true)
	defer batch.Discard()
	h := id.Hash()
	st, err := batch.Transaction(h[:]).Status().Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	st.TxID = id
	st.Code = errors.Delivered
	if st.Received == 0 {
		st.Received = v
	}
	if err := batch.Transaction(h[:]).Status().Put(st); err != nil {
		return errors.UnknownError.Wrap(err)
	}
	return errors.UnknownError.Wrap(batch.Commit())
}
