// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/rand"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// HTLC — hashed time-locked contracts (AIP-48, #3717), shipped in v1.4.6.3.
//
// A send carrying a HashLock produces a SyntheticLockedDeposit instead of a
// normal deposit. The tokens are claimable only by revealing a preimage that
// hashes to the locked value, and are refunded automatically at expiry.
//
// This is two transactions with a dependency between them, which is why it
// needs state the rest of the menu does not: the release cannot be built until
// the lock's SyntheticLockedDeposit exists and its transaction ID is known.
// Locks are parked in a queue by the send, and drained by the release.
//
// Both halves matter for a soak. The send exercises the locked-deposit
// production path; only the release exercises claim validation, preimage
// checking and the credit of locked funds. A run that only sent would leave
// every lock to expire and refund, which is a different code path again.

// hashAlgos is the set to rotate through. All three are exercised because they
// differ in output length (20 vs 32 bytes) and in how the executor validates
// the preimage — HASH160 in particular is the Bitcoin-interop path.
var hashAlgos = []protocol.HashAlgorithm{
	protocol.HashAlgorithmSHA256,
	protocol.HashAlgorithmSHA256D,
	protocol.HashAlgorithmHASH160,
}

const (
	// htlcExpiry is deliberately long. A lock that expires before the release
	// fires is refunded, and the release then fails for a reason that has
	// nothing to do with the code under test. Under chaos a partition can be
	// paused for minutes at a time, so this leaves ample margin.
	htlcExpiry = 30 * time.Minute

	// htlcQueueMax bounds the parked locks. Without a cap, a run where sends
	// outpace releases grows the queue without limit; dropping the oldest is
	// correct because the oldest is the one closest to expiring anyway.
	htlcQueueMax = 64
)

// pendingLock is a lock awaiting release.
type pendingLock struct {
	sendTxID  *url.TxID // the SendTokens that produced the locked deposit
	preimage  []byte
	recipient *identity // who can claim, and therefore who must sign
	created   time.Time
}

// lockQueue holds locks between the send that creates them and the release
// that claims them.
type lockQueue struct {
	mu sync.Mutex
	q  []*pendingLock
}

func (l *lockQueue) push(p *pendingLock) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.q = append(l.q, p)
	if len(l.q) > htlcQueueMax {
		l.q = l.q[len(l.q)-htlcQueueMax:]
	}
}

// pop takes the oldest lock that is not close to expiry. Anything within a
// minute of expiring is discarded rather than returned: building a release for
// it would race the refund and produce a spurious failure.
func (l *lockQueue) pop() *pendingLock {
	l.mu.Lock()
	defer l.mu.Unlock()
	for len(l.q) > 0 {
		p := l.q[0]
		l.q = l.q[1:]
		if time.Since(p.created) < htlcExpiry-time.Minute {
			return p
		}
	}
	return nil
}

// sendTokensHashLock sends ACME under a hash lock, producing a
// SyntheticLockedDeposit rather than a direct deposit.
var sendTokensHashLock = action{
	name: "send-tokens-hashlock", weight: 3, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		from := e.u.randIdentity()
		if from == nil || len(from.tokens) == 0 {
			return nil, errors.NotReady.With("no funded token account")
		}
		// The recipient must be an identity, not a lite account: releasing
		// requires a signer, and the claim is signed by the recipient.
		to := e.u.randIdentity()
		if to == nil || to == from || len(to.tokens) == 0 || to.signer() == nil {
			return nil, errors.NotReady.With("no identity recipient that can sign a release")
		}
		s := from.signer()
		if s == nil {
			return nil, errors.NotReady.With("no signer")
		}

		preimage := make([]byte, 32)
		if _, err := rand.Read(preimage); err != nil {
			return nil, err
		}
		algo := hashAlgos[e.u.intn(len(hashAlgos))]
		expiry := time.Now().Add(htlcExpiry)
		payer := from.tokens[e.u.intn(len(from.tokens))]

		ids, err := e.sign(ctx, s.url, func() txBuilder {
			b := e.build(payer).
				SendTokens(sendAmount, protocol.AcmePrecisionPower).To(to.tokens[0]).
				FinishTransaction()
			switch algo {
			case protocol.HashAlgorithmSHA256D:
				b = b.HashLockSHA256DFromPreimage(preimage, expiry)
			case protocol.HashAlgorithmHASH160:
				b = b.HashLockHASH160FromPreimage(preimage, expiry)
			default:
				b = b.HashLockSHA256FromPreimage(preimage, expiry)
			}
			return b.SignWith(s.url).Version(s.version).
				Timestamp(e.nonce.next()).PrivateKey(from.key())
		})
		if err != nil || len(ids) == 0 {
			return ids, err
		}

		e.locks.push(&pendingLock{
			sendTxID:  ids[0],
			preimage:  preimage,
			recipient: to,
			created:   time.Now(),
		})
		return ids, nil
	},
}

// releaseLocked claims a locked deposit by revealing its preimage.
var releaseLocked = action{
	name: "release-locked", weight: 3, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		p := e.locks.pop()
		if p == nil {
			return nil, errors.NotReady.With("no lock awaiting release")
		}

		// The release names the SyntheticLockedDeposit, not the send that
		// caused it, so the send's produced set has to be resolved first. It
		// is not there immediately — the synthetic has to be produced,
		// delivered and recorded — so a miss is a skip, and the lock goes
		// back on the queue for a later attempt.
		lockedTxID, err := e.findLockedDeposit(ctx, p.sendTxID)
		if err != nil {
			e.locks.push(p)
			return nil, errors.NotReady.WithFormat("locked deposit not visible yet: %w", err)
		}

		s := p.recipient.signer()
		if s == nil {
			return nil, errors.NotReady.With("recipient cannot sign")
		}
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(p.recipient.tokens[0]).
				ReleaseLockedOperation(lockedTxID).WithPreimage(p.preimage).
				SignWith(s.url).Version(s.version).
				Timestamp(e.nonce.next()).PrivateKey(p.recipient.key())
		})
	},
}

// findLockedDeposit resolves the SyntheticLockedDeposit produced by a send.
func (e *env) findLockedDeposit(ctx context.Context, send *url.TxID) (*url.TxID, error) {
	r, err := e.Q.QueryMessage(ctx, send, nil)
	if err != nil {
		return nil, err
	}
	if r.Produced == nil {
		return nil, errors.NotReady.With("no produced transactions")
	}
	for _, p := range r.Produced.Records {
		if p == nil || p.Value == nil {
			continue
		}
		pr, err := e.Q.QueryMessage(ctx, p.Value, nil)
		if err != nil {
			continue
		}
		txn, ok := pr.Message.(interface {
			GetTransaction() *protocol.Transaction
		})
		if !ok || txn.GetTransaction() == nil || txn.GetTransaction().Body == nil {
			continue
		}
		if txn.GetTransaction().Body.Type() == protocol.TransactionTypeSyntheticLockedDeposit {
			return p.Value, nil
		}
	}
	return nil, errors.NotReady.With("no SyntheticLockedDeposit among produced")
}
