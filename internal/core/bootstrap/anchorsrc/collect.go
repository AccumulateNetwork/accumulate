// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A Validator is one validator of the producing partition, reached by name:
// its sequencer, which answers the anchor it produced under a sequence number
// signed with its own key (internal/api/v3, Sequencer.anchorRecord; #4424:
// committee members only).
type Validator struct {
	Name      string
	Sequencer private.Sequencer
}

// Validators finds the producing partition's validators. It never returns the
// node that asks: a joining node's own sequencer answers from the state the
// join exists to fill (#4303).
type Validators interface {
	ValidatorsOf(ctx context.Context, partition *url.URL) ([]Validator, error)
}

// A Collector gathers a partition's OWN anchors from its validators, each
// signed by the validator that answers, and records one when distinct members
// of the partition's validator set reaching its threshold have signed it
// (executor spec, "Sync", "The algorithm", step 3: "the anchor is the
// partition's own"). The anchor of block B is produced as B closes, so it is
// there to collect as soon as B is; the copy in the Directory's pool arrives
// only after the Directory executes it, which is far too late to match a
// partition that moves every block (Paul, 2026-09-25).
//
// A partition's own pool does not hold its own anchors with their signatures
// -- a BVN's holds none of them; only the Directory, which anchors to itself,
// holds its own -- so there is nothing to read them from but the validators.
// The Directory's own anchors are collected the same way, from the
// Directory's validators.
//
// Verification is VerifyQuorum: distinct members of the set
// this node trusts, to that set's threshold, each signature checked against
// the anchor it carries.
type Collector struct {
	// Producer is the partition whose anchors are collected.
	Producer *url.URL

	// Authority is the validator sets anchors are checked against. The
	// collector only reads it.
	Authority *Authority

	// Validators finds the producer's validators.
	Validators Validators

	// Ledgers reaches each of the producer's peers, by name, for its anchor
	// ledger, which says the sequence number of the last anchor produced. It
	// positions the first read and nothing else: an anchor is taken on its
	// signatures. The LOWEST number any peer answers is taken, so one peer
	// cannot place the read above every anchor there is (#4438 F4); a peer
	// that answers low only makes the read start earlier.
	Ledgers func(ctx context.Context) ([]AccountReader, error)

	// OnAnchor is called for every anchor a quorum signed, with the block
	// it anchors and the root that block committed.
	OnAnchor func(partition *url.URL, block uint64, root [32]byte)

	// OnRefused is called when a produced anchor could not be taken.
	OnRefused func(block uint64, err error)

	// OnRootChain is called with the same anchors as OnAnchor, with the root
	// chain position and anchor the anchor carries: what a receipt from an
	// entry of this partition's chains ends at.
	OnRootChain func(block, rootIndex uint64, rootAnchor [32]byte)

	// OnAsk is called before each question a read asks a peer: the anchor
	// number and the validator asked, or zero and the peer asked for the
	// newest number. A read asks one validator after another and logs
	// nothing that answers, so this is the one way to say which question a
	// read that lasts is on (#4484).
	OnAsk func(anchor uint64, who string)

	// OnCall, if set, is told how long each call took: to a validator for an
	// anchor, or, with anchor zero, to a peer for its ledger's newest number.
	// A read that lasts is one of these, and this says which (#4524).
	OnCall func(anchor uint64, who string, took time.Duration)

	mu     sync.Mutex
	next   uint64 // the next sequence number to collect; zero until positioned
	newest uint64 // the newest number the ledger named when the read was positioned
	stall  *Stall
}

// collectBackfill is how many anchors before the newest the first read starts
// at. A node restarted a few blocks behind may match its own state as it
// stands (#4411), and only an anchor of a block at or before the newest can
// say so.
const collectBackfill = 16

// maxAnchorsPerRead bounds one Read. The number to collect from moves only by
// what was collected, so a node far behind catches up over several reads.
const maxAnchorsPerRead = 32

// NewCollector constructs a Collector.
// An AccountReader is one peer's account query.
type AccountReader interface {
	QueryAccount(ctx context.Context, scope *url.URL, query *api.DefaultQuery) (*api.AccountRecord, error)
}

func NewCollector(producer *url.URL, authority *Authority, validators Validators, ledgers func(ctx context.Context) ([]AccountReader, error)) (*Collector, error) {
	switch {
	case producer == nil:
		return nil, errors.BadRequest.With("anchorsrc.NewCollector: a producer partition is required")
	case authority == nil:
		return nil, errors.BadRequest.With("anchorsrc.NewCollector: an authority is required — an unverified root is not a root")
	case validators == nil:
		return nil, errors.BadRequest.With("anchorsrc.NewCollector: the producer's validators are required")
	case ledgers == nil:
		return nil, errors.BadRequest.With("anchorsrc.NewCollector: the producer's peers are required")
	}
	if _, ok := protocol.ParsePartitionUrl(producer); !ok {
		return nil, errors.BadRequest.WithFormat("%v is not a partition", producer)
	}
	return &Collector{Producer: producer, Authority: authority, Validators: validators, Ledgers: ledgers}, nil
}

// Rewind positions the next read at the newest anchor again. The join calls it
// when the trusted sets move: anchors refused under the old set are asked for
// again under the new one.
func (c *Collector) Rewind() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next = 0
}

// Stalled reports the anchor the collector is held at, if it is: one some
// validator produced and answered, and that no quorum of the set signed this
// read. Stall.Entry is the anchor's sequence number.
func (c *Collector) Stalled() (Stall, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stall == nil {
		return Stall{}, false
	}
	st := *c.stall
	st.Asked = append([]string(nil), st.Asked...)
	return st, true
}

// Read collects every anchor produced since the last read, in sequence
// order, up to maxAnchorsPerRead, calling OnAnchor for each a quorum signed.
// It stops at the first number no validator has produced yet, and at the
// first one no quorum signed, which the next read asks for again.
func (c *Collector) Read(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.next == 0 {
		last, err := c.lastProduced(ctx)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if last == 0 {
			return nil // Nothing produced yet
		}
		c.next, c.newest = 1, last
		if last > collectBackfill {
			c.next = last - collectBackfill
		}
	}

	vals, err := c.Validators.ValidatorsOf(ctx, c.Producer)
	if err != nil {
		return errors.UnknownError.WithFormat("find %v's validators: %w", c.Producer, err)
	}
	if len(vals) == 0 {
		return errors.NotReady.WithFormat("no validator of %v other than this node was found", c.Producer)
	}

	askedLedgers := false
	for i := 0; i < maxAnchorsPerRead && ctx.Err() == nil; i++ {
		done, answered, notYet, err := c.collect(ctx, vals, c.next)
		if err != nil {
			return err
		}
		if !done && !answered && notYet {
			// Not produced yet: a wait, not a number to skip. A live
			// partition's ledgers name an anchor before its validators
			// serve it; skipping it and asking the ledgers again for every
			// number ran the collector past every signed anchor, asking
			// some 39 times a second (#4524). The next read asks again.
			return nil
		}
		if !done && !answered && c.next >= c.newest {
			if askedLedgers {
				return nil // The ledgers are asked once a read
			}
			askedLedgers = true
			// Not answered, at or past the newest the ledgers named when
			// the read was positioned: not produced yet -- or no longer
			// held, if the producer has moved on since. A collector read
			// less often than the producer anchors falls out of the
			// validators' caches, and with a newest that never moved it
			// waited on a number no one would answer again: the BVN side
			// of run 20260928T051402Z sat at one anchor for nine hours
			// (#4442). The ledgers are asked again; a number below what
			// they name now is skipped, to the backfill below it.
			last, err := c.lastProduced(ctx)
			if err != nil || last <= c.next {
				return nil
			}
			c.newest = last
			if last > collectBackfill && last-collectBackfill > c.next {
				c.next = last - collectBackfill
				continue
			}
		}
		if !done && answered {
			// Held at an anchor no quorum signed: the next read asks again.
			return nil
		}
		// Taken, or older than the newest and answered by nobody: a
		// number the validators no longer hold is not a wait.
		c.next++
	}
	return nil
}

// collect asks every validator for anchor number n and takes it when a quorum
// signed it. It reports whether it took it, and whether any validator answered
// it at all: one that none answered is not produced yet, or no longer held.
func (c *Collector) collect(ctx context.Context, vals []Validator, n uint64) (taken, answered, notYet bool, err error) {
	src := c.Producer.JoinPath(protocol.AnchorPool)
	dst := protocol.DnUrl()
	producer, _ := protocol.ParsePartitionUrl(c.Producer)

	// Answers for one number from honest validators are one anchor; a
	// validator that answers another body is a group of its own, and
	// signatures are counted within a group, never across.
	groups := map[[32]byte]*api.MessageRecord[*messaging.TransactionMessage]{}
	var order [][32]byte
	var asked []string
	var lastErr error
	produced := false
	for _, v := range vals {
		asked = append(asked, v.Name)
		if c.OnAsk != nil {
			c.OnAsk(n, v.Name)
		}
		start := time.Now()
		ans, err := v.Sequencer.Sequence(ctx, src, dst, n, private.SequenceOptions{})
		if c.OnCall != nil {
			c.OnCall(n, v.Name, time.Since(start))
		}
		if err != nil {
			if ctx.Err() != nil {
				return false, false, false, errors.UnknownError.Wrap(ctx.Err())
			}
			if errors.Is(err, errors.NotReady) {
				notYet = true
			}
			lastErr = err
			continue
		}
		rec, err := anchorOf(ans)
		if err == nil && rec.Sequence.Number != n {
			err = errors.Conflict.WithFormat("asked for anchor %d, it answered anchor %d", n, rec.Sequence.Number)
		}
		if err == nil {
			err = c.isOwn(rec)
		}
		if err != nil {
			lastErr = errors.UnknownError.WithFormat("%s: %w", v.Name, err)
			continue
		}
		produced = true
		h := rec.Sequence.Hash()
		g, ok := groups[h]
		if !ok {
			groups[h] = rec
			order = append(order, h)
			continue
		}
		if rec.Signatures != nil {
			if g.Signatures == nil {
				g.Signatures = new(api.RecordRange[*api.SignatureSetRecord])
			}
			g.Signatures.Records = append(g.Signatures.Records, rec.Signatures.Records...)
			g.Signatures.Total += rec.Signatures.Total
		}
	}
	if !produced {
		// No validator answered an anchor under this number: not produced
		// yet, no longer held, or not reachable this read. At the newest
		// number the ledgers named it is said: a position no validator
		// can answer is where the collector is held (#4438 F4).
		if n == c.newest {
			c.stall = &Stall{Entry: n, Asked: asked, Err: errors.NotFound.WithFormat(
				"no validator answers anchor %d, the newest the peers' ledgers name: %v", n, lastErr)}
		}
		return false, false, notYet, nil
	}

	// Every group a quorum signed is observed, not only the first: a group
	// that verifies and is not the partition's own anchor would otherwise
	// hide the one that is (#4438 F2).
	for _, h := range order {
		rec := groups[h]
		pa := rec.Message.Transaction.Body.(protocol.AnchorBody).GetPartitionAnchor()
		err := VerifyQuorum(c.Authority, producer, rec)
		if err != nil {
			lastErr = err
			if c.OnRefused != nil {
				c.OnRefused(pa.MinorBlockIndex, err)
			}
			continue
		}
		taken = true
		if c.OnAnchor != nil {
			c.OnAnchor(c.Producer, pa.MinorBlockIndex, pa.StateTreeAnchor)
		}
		if c.OnRootChain != nil {
			c.OnRootChain(pa.MinorBlockIndex, pa.RootChainIndex, pa.RootChainAnchor)
		}
	}
	if taken {
		c.stall = nil
		if n > c.newest {
			c.newest = n
		}
		return true, true, notYet, nil
	}

	c.stall = &Stall{Entry: n, Asked: asked, Err: lastErr}
	return false, true, notYet, nil
}

// isOwn refuses an answer that is not the producer's own anchor: sent by the
// producer, to the Directory, carrying the producer's partition anchor in the
// producer's kind of body. Validator keys sit on the Directory and on their
// BVN at once, so the Directory's anchor, relayed under a BVN's name, would
// otherwise pass the BVN's threshold (#4438 F2).
func (c *Collector) isOwn(rec *api.MessageRecord[*messaging.TransactionMessage]) error {
	if rec.Sequence.Source == nil || !rec.Sequence.Source.RootIdentity().Equal(c.Producer) {
		return errors.Conflict.WithFormat("the anchor was sent by %v, not %v", rec.Sequence.Source, c.Producer)
	}
	if rec.Sequence.Destination == nil || !rec.Sequence.Destination.RootIdentity().Equal(protocol.DnUrl()) {
		return errors.Conflict.WithFormat("the anchor was sent to %v, not the Directory", rec.Sequence.Destination)
	}
	body := rec.Message.Transaction.Body
	if protocol.DnUrl().Equal(c.Producer) {
		if _, ok := body.(*protocol.DirectoryAnchor); !ok {
			return errors.Conflict.WithFormat("the Directory's anchor is a %v", body.Type())
		}
	} else if _, ok := body.(*protocol.BlockValidatorAnchor); !ok {
		return errors.Conflict.WithFormat("a BVN's anchor is a %v", body.Type())
	}
	pa := body.(protocol.AnchorBody).GetPartitionAnchor()
	if pa.Source == nil || !pa.Source.Equal(c.Producer) {
		return errors.Conflict.WithFormat("the anchor is %v's, not %v's", pa.Source, c.Producer)
	}
	return nil
}

// anchorOf is the anchor a sequencer answered, or why the answer is not one.
func anchorOf(ans *api.MessageRecord[messaging.Message]) (*api.MessageRecord[*messaging.TransactionMessage], error) {
	if ans == nil || ans.Message == nil || ans.Sequence == nil {
		return nil, errors.BadRequest.With("the validator answered no anchor")
	}
	rec, err := api.MessageRecordAs[*messaging.TransactionMessage](ans)
	if err != nil || rec == nil || rec.Message == nil || rec.Message.Transaction == nil || rec.Message.Transaction.Body == nil {
		return nil, errors.BadRequest.WithFormat("the validator answered a %v, not an anchor transaction", ans.Message.Type())
	}
	body, ok := rec.Message.Transaction.Body.(protocol.AnchorBody)
	if !ok || body.GetPartitionAnchor() == nil {
		return nil, errors.BadRequest.WithFormat("the validator answered a %v, not an anchor", rec.Message.Transaction.Body.Type())
	}
	// The sequenced message a signature covers is the one served, with the
	// message it carries; the record's copy is authoritative.
	seq := *rec.Sequence
	seq.Message = rec.Message
	rec.Sequence = &seq
	return rec, nil
}

// lastProduced is the sequence number of the producer's newest anchor, the
// lowest any of its peers' anchor ledgers names.
func (c *Collector) lastProduced(ctx context.Context) (uint64, error) {
	u := c.Producer.JoinPath(protocol.AnchorPool)
	peers, err := c.Ledgers(ctx)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("find %v's peers: %w", c.Producer, err)
	}
	var low uint64
	var found bool
	var last error
	for _, p := range peers {
		if c.OnAsk != nil {
			c.OnAsk(0, fmt.Sprint(p))
		}
		start := time.Now()
		rec, err := p.QueryAccount(ctx, u, nil)
		if c.OnCall != nil {
			c.OnCall(0, fmt.Sprint(p), time.Since(start))
		}
		if err != nil {
			last = err
			continue
		}
		ledger, ok := rec.Account.(*protocol.AnchorLedger)
		if !ok {
			last = errors.Conflict.WithFormat("a peer served %v as %v, not an anchor ledger", u, rec.Account.Type())
			continue
		}
		if !found || ledger.MinorBlockSequenceNumber < low {
			low, found = ledger.MinorBlockSequenceNumber, true
		}
	}
	if !found {
		return 0, errors.UnknownError.WithFormat("no peer served %v: %w", u, last)
	}
	return low, nil
}

// String names the collector in a log line.
func (c *Collector) String() string { return fmt.Sprintf("%v's validators", c.Producer) }
