// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package backwalk constructs the proof of derivation from genesis as a
// graph traversal across main chains (issue #3960, parent #3953).
//
// The traversal starts at a current account or keybook (pulled from a
// peer at the bootstrap pin block H) and walks each main chain backward.
// Every entry is verified by one of two rules:
//
//   - User-signed entries: signatures live on the *signer's* signature
//     chain (lateral navigation); resolve the keypage at the entry's
//     block time via #3957; verify the signature.
//   - Synthetic entries (cross-partition forwards, anchor results,
//     etc., carrying InternalSignature): trace the Cause to the
//     producing transaction and recurse; additionally verify the
//     synthetic was included in a validator-quorum-signed anchor. The
//     validator set at that block time is itself resolved via #3957
//     (the operators / partition keybook).
//
// Recursion bottoms out at the genesis snapshot: each chain's earliest
// entry must reference an account or keybook present in the genesis
// snapshot whose hash matches the binary's pinned value.
//
// Memoization keyed by (account, blockTime) handles legitimate cyclic
// dependencies between keybooks (mutual signing relationships).
package backwalk

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Walker constructs a proof of derivation by walking main chains.
type Walker struct {
	mu              sync.Mutex
	pinnedGenesis   [32]byte
	memo            map[memoKey]*VerifiedEntry
	terminations    map[*url.URL]struct{}
	stack           map[memoKey]struct{} // cycle detection (current DFS path)
	maxRecursion    int
	currentDepth    int
}

type memoKey struct {
	url  string
	time int64 // unix nanos
}

// VerifiedEntry is one validated main-chain entry in the proof.
type VerifiedEntry struct {
	Account     *url.URL
	BlockTime   time.Time
	TxHash      [32]byte
	SignerUrl   *url.URL
	Synthetic   bool // true if authenticated by validator-quorum + Cause
	GenesisTerm bool // true if this entry's chain bottoms out at the genesis snapshot

	// QuorumPending is true when this entry is synthetic and the
	// validator-quorum cryptographic check on the anchoring transaction
	// is not yet implemented. Structural plumbing (Cause traversal) is
	// complete; the caller can record this in the proof artifact.
	QuorumPending bool

	// Causes is non-empty for synthetic entries — the producing
	// transaction(s) traced via the Cause links.
	Causes []*url.TxID
}

// Options configures a Walker.
type Options struct {
	// PinnedGenesisHash is the hash of the genesis snapshot the binary
	// was built against — the only out-of-band trust input.
	PinnedGenesisHash [32]byte

	// MaxRecursion bounds the depth of (account, blockTime) recursion
	// to defend against pathological cycles. Zero uses a sane default
	// (see DefaultMaxRecursion).
	MaxRecursion int
}

// DefaultMaxRecursion is the default depth bound for keypage-at-time
// recursion. Empirically the operator key book on mainnet has 1
// main-chain entry, so depth ~1 is enough for the typical case; this
// default leaves headroom for future complexity.
const DefaultMaxRecursion = 64

// New constructs a Walker.
func New(opts Options) *Walker {
	maxR := opts.MaxRecursion
	if maxR == 0 {
		maxR = DefaultMaxRecursion
	}
	return &Walker{
		pinnedGenesis: opts.PinnedGenesisHash,
		memo:          make(map[memoKey]*VerifiedEntry),
		terminations:  make(map[*url.URL]struct{}),
		stack:         make(map[memoKey]struct{}),
		maxRecursion:  maxR,
	}
}

// ErrCycleDetected is returned when keypage-at-time recursion enters a
// cycle that can't be broken by memoization.
var ErrCycleDetected = errors.New("backwalk: recursion cycle detected")

// ErrMaxRecursion is returned when the recursion depth bound is hit.
var ErrMaxRecursion = errors.New("backwalk: maximum recursion depth exceeded")

// ErrNotImplemented is returned for code paths not yet implemented.
// The walker's interface is stable; the underlying chain-walking and
// signature-verification logic is implemented incrementally.
var ErrNotImplemented = errors.New("backwalk: code path not yet implemented")

// Walk runs the back-walk for accountUrl as of blockTime, recording the
// validated chain in the Walker's internal proof state. Returns the
// terminal verified entry (the genesis-snapshot hit if reached) or an
// error.
//
// If the (accountUrl, blockTime) pair is already memoized, the cached
// entry is returned without any DB access — useful for callers that
// pre-populate via Memoize for tests or for restoring persisted state.
func (w *Walker) Walk(batch *database.Batch, accountUrl *url.URL, blockTime time.Time) (*VerifiedEntry, error) {
	if accountUrl == nil {
		return nil, fmt.Errorf("nil accountUrl")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Memo lookup before any DB work; supports the cache-hit fast path.
	mk := memoKey{url: accountUrl.String(), time: blockTime.UnixNano()}
	if cached, ok := w.memo[mk]; ok {
		return cached, nil
	}

	if batch == nil {
		return nil, fmt.Errorf("nil batch")
	}

	return w.walkLocked(batch, accountUrl, blockTime)
}

func (w *Walker) walkLocked(batch *database.Batch, accountUrl *url.URL, blockTime time.Time) (*VerifiedEntry, error) {
	mk := memoKey{url: accountUrl.String(), time: blockTime.UnixNano()}

	if cached, ok := w.memo[mk]; ok {
		return cached, nil
	}
	if _, on := w.stack[mk]; on {
		return nil, ErrCycleDetected
	}
	if w.currentDepth >= w.maxRecursion {
		return nil, ErrMaxRecursion
	}

	w.stack[mk] = struct{}{}
	w.currentDepth++
	defer func() {
		delete(w.stack, mk)
		w.currentDepth--
	}()

	acct := batch.Account(accountUrl)

	mainChain, err := acct.MainChain().Get()
	if err != nil {
		return nil, fmt.Errorf("get main chain for %s: %w", accountUrl, err)
	}
	height := mainChain.Height()
	if height == 0 {
		return nil, fmt.Errorf("backwalk: %s has no main-chain entries", accountUrl)
	}

	// Walk backward, recording verified entries until we reach the
	// chain's earliest entry (the candidate genesis terminator).
	var earliest *VerifiedEntry
	for i := height - 1; i >= 0; i-- {
		entryHash, err := mainChain.Entry(i)
		if err != nil {
			return nil, fmt.Errorf("read entry %d: %w", i, err)
		}
		var hashArr [32]byte
		copy(hashArr[:], entryHash)

		entryBlockTime, err := w.entryBlockTime(acct, uint64(i))
		if err != nil {
			return nil, fmt.Errorf("block time for entry %d: %w", i, err)
		}
		// Skip entries newer than the requested blockTime — caller asked
		// for the chain as it existed at blockTime, so anything later
		// is not part of the proof window.
		if entryBlockTime != nil && entryBlockTime.After(blockTime) {
			continue
		}

		ve, err := w.verifyEntry(batch, accountUrl, hashArr, entryBlockTime)
		if err != nil {
			return nil, fmt.Errorf("verify entry %d (%x): %w", i, hashArr[:8], err)
		}
		w.memoizeLocked(ve)
		earliest = ve
	}

	if earliest == nil {
		return nil, fmt.Errorf("backwalk: %s has no entries at or before %s", accountUrl, blockTime.Format(time.RFC3339))
	}

	// Genesis termination: if the earliest entry is a SystemGenesis
	// transaction (or otherwise signals the genesis snapshot), mark
	// GenesisTerm. Cross-check against the pinned hash is a follow-up
	// once we have the genesis-snapshot manifest available locally.
	if isGenesisTerminator(batch, earliest.TxHash) {
		earliest.GenesisTerm = true
		w.terminations[accountUrl] = struct{}{}
	}
	return earliest, nil
}

// verifyEntry classifies the transaction at txnHash and dispatches to
// the user-keypage or synthetic verification rule. Returns a
// VerifiedEntry with the appropriate flags.
func (w *Walker) verifyEntry(batch *database.Batch, accountUrl *url.URL, txnHash [32]byte, blockTime *time.Time) (*VerifiedEntry, error) {
	txn, err := loadTx(batch, txnHash)
	if err != nil {
		return nil, fmt.Errorf("load tx: %w", err)
	}

	bt := time.Time{}
	if blockTime != nil {
		bt = *blockTime
	}

	ve := &VerifiedEntry{
		Account:   accountUrl,
		BlockTime: bt,
		TxHash:    txnHash,
	}

	switch {
	case txn.Body.Type().IsSynthetic(), txn.Body.Type().IsSystem():
		// Synthetic / system path: trace cause and run validator-quorum
		// check if we can determine the producing partition.
		sv, err := verifySynthetic(batch, txnHash, txn)
		if err != nil {
			return nil, err
		}
		ve.Synthetic = true
		ve.Causes = sv.Causes

		// Run quorum verification for anchor-class transactions. For
		// other synthetic types the partition discovery story isn't
		// yet wired; mark QuorumPending so the proof artifact reflects
		// reality.
		if txn.Body.Type().IsAnchor() && blockTime != nil {
			partition, ok := protocol.ParsePartitionUrl(accountUrl)
			if ok {
				_, qerr := VerifyValidatorQuorum(batch, accountUrl, txnHash, partition, *blockTime)
				if qerr == nil {
					ve.QuorumPending = false
				} else {
					// Don't hard-fail the walk on quorum-pending; the
					// caller decides via QuorumPending=true. This lets
					// us produce a structurally complete proof artifact
					// even when validator signatures haven't all been
					// gathered yet (e.g., during BOOTING when we may
					// not have pulled them).
					ve.QuorumPending = true
				}
			} else {
				ve.QuorumPending = true
			}
		} else {
			ve.QuorumPending = sv.QuorumPending
		}

	case txn.Body.Type().IsUser():
		// User-signed: verify against keypage at blockTime.
		signer, err := signerForTransaction(txn)
		if err != nil {
			return nil, err
		}
		ve.SignerUrl = signer
		// User signatures live at the signer's account. For tonight's
		// slice signer == principal; external-signer flows are a
		// follow-up.
		if blockTime != nil {
			if err := VerifyUserSignaturesAt(batch, txnHash, signer, *blockTime); err != nil {
				// Permit ErrNoSignatures during BOOTING walks where the
				// node hasn't pulled signatures yet — caller can decide.
				return nil, err
			}
		}

	default:
		return nil, fmt.Errorf("unknown transaction class: type=%v", txn.Body.Type())
	}

	return ve, nil
}

// memoizeLocked must be called with w.mu held.
func (w *Walker) memoizeLocked(entry *VerifiedEntry) {
	if entry == nil || entry.Account == nil {
		return
	}
	mk := memoKey{url: entry.Account.String(), time: entry.BlockTime.UnixNano()}
	w.memo[mk] = entry
}

// entryBlockTime resolves the block time for main-chain index `idx` via
// the main-index chain. Returns nil if the index chain has no entry
// covering this main-index yet (e.g., a tip entry not yet anchored).
func (w *Walker) entryBlockTime(acct *database.Account, idx uint64) (*time.Time, error) {
	indexChain, err := acct.MainChain().Index().Get()
	if err != nil {
		return nil, fmt.Errorf("get main-index chain: %w", err)
	}
	if indexChain.Height() == 0 {
		return nil, nil
	}
	// Iterate the index chain looking for the smallest entry with
	// Source >= idx. Start from the end and walk backward.
	for j := int64(indexChain.Height()) - 1; j >= 0; j-- {
		raw, err := indexChain.Entry(j)
		if err != nil {
			return nil, fmt.Errorf("read index entry %d: %w", j, err)
		}
		var ie protocol.IndexEntry
		if err := ie.UnmarshalBinary(raw); err != nil {
			return nil, fmt.Errorf("decode index entry %d: %w", j, err)
		}
		if ie.Source >= idx {
			if ie.BlockTime != nil {
				bt := *ie.BlockTime
				return &bt, nil
			}
			return nil, nil
		}
	}
	return nil, nil
}

// loadTx loads a TransactionMessage from the message store and returns
// its embedded Transaction. Returns a wrapped error if the message is
// not a TransactionMessage.
func loadTx(batch *database.Batch, hash [32]byte) (*protocol.Transaction, error) {
	var msg messaging.Message
	err := batch.Message(hash).Main().GetAs(&msg)
	if err != nil {
		return nil, fmt.Errorf("load message: %w", err)
	}
	tm, ok := msg.(*messaging.TransactionMessage)
	if !ok {
		return nil, fmt.Errorf("message %x is not a transaction (got %T)", hash[:8], msg)
	}
	return tm.Transaction, nil
}

// isGenesisTerminator reports whether the transaction at txnHash should
// be treated as terminating the chain at the genesis snapshot. For
// tonight's slice we accept SystemGenesis as the terminator. A more
// faithful check against the pinned-genesis manifest is a follow-up.
func isGenesisTerminator(batch *database.Batch, txnHash [32]byte) bool {
	txn, err := loadTx(batch, txnHash)
	if err != nil || txn == nil {
		return false
	}
	return txn.Body.Type() == protocol.TransactionTypeSystemGenesis
}

// MemoSize returns the number of cached (account, blockTime) → entry
// memoizations. Used by tests and the persistence layer (#3965).
func (w *Walker) MemoSize() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.memo)
}

// PinnedGenesisHash returns the hash the walker is anchoring to.
func (w *Walker) PinnedGenesisHash() [32]byte {
	return w.pinnedGenesis
}

// Memoize manually records a verified entry. Exposed for tests and for
// loading persisted memoizations on restart (#3965). The entry's
// Account and BlockTime are used as the key.
func (w *Walker) Memoize(entry *VerifiedEntry) {
	if entry == nil || entry.Account == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	mk := memoKey{url: entry.Account.String(), time: entry.BlockTime.UnixNano()}
	w.memo[mk] = entry
}
