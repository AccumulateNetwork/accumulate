// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// action is one flavour of transaction the generator can emit.
type action struct {
	name   string
	weight int
	// needsIdentity marks actions that require at least one fully built
	// identity. Early in a run there are none, so those are skipped.
	needsIdentity bool
	// expectFail marks actions that deliberately submit a transaction that is
	// meant to fail (forcing a refund). They are counted but NOT followed for
	// delivery, so they never count toward -max-stranded.
	expectFail bool
	run        func(ctx context.Context, e *env) ([]*url.TxID, error)
}

// menu is the full set of user transaction types the generator exercises.
// Weights are relative; value transfers dominate because that is what real
// load looks like, but every structural transaction fires regularly.
var menu = []action{
	sendTokensLite, sendTokensADI, addCreditsLite, addCreditsPage,
	writeDataADI, writeDataLite, burnTokens, burnTokensADI, burnCredits, transferCredits,
	setThreshold, addPageKey, updatePageKey, removePageKey,
	updateAccountAuth, lockAccount,
	addBook, addAccount,
	addToken, issueTokens, sendTokensCustom, burnTokensCustom,
	// Refund/failure paths (deliberately fail; see failures.go).
	sendTokensToVoid, overSpendTokens, overBurnTokens,
	createSubAdiOnVoid, writeDataToVoid,
}

// pick chooses the next action, skipping any that cannot run yet. Weights come
// through weightOf so runtime overrides from the control API apply; a weight of
// 0 removes an action from the draw entirely.
func (e *env) pick() action {
	haveIdentity := e.u.randIdentity() != nil

	total := 0
	for _, a := range menu {
		if a.needsIdentity && !haveIdentity {
			continue
		}
		total += e.weightOf(a)
	}
	if total == 0 {
		return sendTokensLite
	}

	n := e.u.intn(total)
	for _, a := range menu {
		if a.needsIdentity && !haveIdentity {
			continue
		}
		w := e.weightOf(a)
		if n < w {
			return a
		}
		n -= w
	}
	return sendTokensLite
}

// --- value movement -------------------------------------------------------

// sendTokensLite cascades ACME between lite accounts. A funded lite distributes
// to another lite (occasionally a brand new one, so the set keeps growing), so
// value moves account -> account -> account across partitions. The treasury only
// seeds: when no funded lite exists yet, it sends a larger amount to one lite so
// that lite can then relay many times.
var sendTokensLite = action{
	name: "send-tokens-lite", weight: 18,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		to := e.u.randLite()
		if to == nil || (e.u.intn(8) == 0 && !e.u.atLiteCap()) {
			to = newLiteAccount(e.u.rng)
			e.u.addLite(to)
		}

		// Cascade: a funded, ready lite distributes to another lite.
		if src := e.u.randSourceLite(); src != nil && src != to {
			e.u.markFunded(to)
			return e.sign(ctx, src.id, func() txBuilder {
				return e.build(src).
					SendTokens(sendAmount, protocol.AcmePrecisionPower).To(to.acct).
					SignWith(src.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(src.key)
			})
		}

		// Seed: no funded lite yet — the treasury seeds this one so the cascade
		// can start from it.
		if !e.canPay(treasuryFloorUnits) {
			return nil, errors.NotReady.With("treasury is low on credits")
		}
		e.u.markFunded(to)
		t := e.treasury
		return e.submitAsTreasury(ctx, func() txBuilder {
			return e.build(t).
				SendTokens(liteSeed, protocol.AcmePrecisionPower).To(to.acct).
				SignWith(t.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(t.key)
		})
	},
}

// sendTokensADI moves ACME out of an ADI token account.
var sendTokensADI = action{
	name: "send-tokens-adi", weight: 15, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		from := e.u.randIdentity()
		if from == nil || len(from.tokens) == 0 {
			return nil, errors.NotReady.With("no funded token account")
		}
		var to *url.URL
		if other := e.u.randIdentity(); other != nil && len(other.tokens) > 0 && other != from {
			to = other.tokens[0] // prefer another token account — the cascade
		} else if l := e.u.randLite(); l != nil {
			to = l.acct
			e.u.markFunded(l) // it now holds ACME and can relay
		} else {
			return nil, errors.NotReady.With("nowhere to send")
		}
		s := from.signer()
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(from.tokens[e.u.intn(len(from.tokens))]).
				SendTokens(sendAmount, protocol.AcmePrecisionPower).To(to).
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(from.key())
		})
	},
}

// addCreditsLite converts ACME into credits on a lite identity.
var addCreditsLite = action{
	name: "add-credits-lite", weight: 8,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		if !e.canPay(treasuryFloorUnits) {
			return nil, errors.NotReady.With("treasury is low on credits")
		}
		to := e.u.randLite()
		if to == nil {
			return nil, errors.NotReady.With("no lite account")
		}
		t := e.treasury
		return e.submitAsTreasury(ctx, func() txBuilder {
			return e.build(t).
				AddCredits().WithOracle(e.oracle).Purchase(liteCreditGrant).To(to.id).
				SignWith(t.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(t.key)
		})
	},
}

// addCreditsPage tops a key page up, which also keeps long runs solvent.
// addCreditsPage has an identity buy credits for one of its own key pages, paid
// from its OWN ACME token account. The credit purchase burns ACME to acc://ACME
// on the DN, so this produces a synthetic sourced from wherever the identity
// lives — populating BVN->DN traffic from every partition, not just the treasury.
var addCreditsPage = action{
	name: "add-credits-page", weight: 8, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil {
			return nil, errors.NotReady.With("no identity")
		}
		s := a.signer()
		p := e.u.randPage(a)
		if s == nil || p == nil || len(a.tokens) == 0 {
			return nil, errors.NotReady.With("no funded identity to buy credits")
		}
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(a.tokens[0]).
				AddCredits().WithOracle(e.oracle).Purchase(adiCreditGrant).To(p.url).
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
	},
}

var burnTokens = action{
	name: "burn-tokens", weight: 3,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		if !e.canPay(treasuryFloorUnits) {
			return nil, errors.NotReady.With("treasury is low on credits")
		}
		t := e.treasury
		return e.submitAsTreasury(ctx, func() txBuilder {
			return e.build(t).
				BurnTokens(sendAmount, protocol.AcmePrecisionPower).
				SignWith(t.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(t.key)
		})
	},
}

// burnTokensADI burns ACME from an identity's own token account. The burn routes
// to acc://ACME on the DN, so — like addCreditsPage — it sources BVN->DN traffic
// from every partition, not just the treasury's.
var burnTokensADI = action{
	name: "burn-tokens-adi", weight: 4, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil || len(a.tokens) == 0 || a.signer() == nil {
			return nil, errors.NotReady.With("no funded token account")
		}
		s := a.signer()
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(a.tokens[e.u.intn(len(a.tokens))]).
				BurnTokens(sendAmount, protocol.AcmePrecisionPower).
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
	},
}

var burnCredits = action{
	name: "burn-credits", weight: 2, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil || a.signer() == nil {
			return nil, errors.NotReady.With("no signer")
		}
		s := a.signer()
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(s).
				BurnCredits(1).
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
	},
}

// transferCredits moves credits between key pages. Credits cannot leave the
// ADI that holds them, so both pages must belong to the same identity.
var transferCredits = action{
	name: "transfer-credits", weight: 3, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil || a.signer() == nil {
			return nil, errors.NotReady.With("no identity")
		}
		_, to := e.u.mutablePage(a)
		if to == nil {
			return nil, errors.NotReady.With("no second page to transfer to")
		}
		s := a.signer()
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(s).
				TransferCredits(1).To(to.url).
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
	},
}

// --- data -----------------------------------------------------------------

var writeDataADI = action{
	name: "write-data-adi", weight: 12, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil || len(a.data) == 0 || a.signer() == nil {
			return nil, errors.NotReady.With("no data account")
		}
		s := a.signer()
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(a.data[e.u.intn(len(a.data))]).
				WriteData().DoubleHash("loadgen", fmt.Sprintf("%d", e.u.intn(1<<30))).
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
	},
}

// writeDataLite writes to a lite data account, whose address is derived from
// the entry itself.
var writeDataLite = action{
	name: "write-data-lite", weight: 8,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		if !e.canPay(treasuryFloorUnits) {
			return nil, errors.NotReady.With("treasury is low on credits")
		}
		entry := &protocol.DoubleHashDataEntry{Data: [][]byte{
			[]byte("loadgen"),
			[]byte(fmt.Sprintf("%d", e.u.intn(1<<20))),
		}}
		lda, err := protocol.LiteDataAddress(protocol.ComputeLiteDataAccountId(entry))
		if err != nil {
			return nil, err
		}
		t := e.treasury
		return e.submitAsTreasury(ctx, func() txBuilder {
			return e.build(t).
				WriteData().Entry(entry).To(lda).
				SignWith(t.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(t.key)
		})
	},
}

// --- key page shape -------------------------------------------------------
//
// These all target a page other than the identity's signer, and are signed by
// the signer, which outranks them. That keeps every generated page reachable
// no matter what threshold or key set it ends up with.

var setThreshold = action{
	name: "set-threshold", weight: 4, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil {
			return nil, errors.NotReady.With("no identity")
		}
		_, p := e.u.mutablePage(a)
		if p == nil {
			return nil, errors.NotReady.With("no mutable page")
		}
		s := a.signer()
		threshold := uint64(1 + e.u.intn(len(p.keys)))
		ids, err := e.sign(ctx, s.url, func() txBuilder {
			return e.build(p).
				UpdateKeyPage().SetThreshold(threshold).
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
		if err != nil {
			return nil, err
		}
		e.u.mu.Lock()
		p.threshold, p.version = threshold, p.version+1
		e.u.mu.Unlock()
		return ids, nil
	},
}

var addPageKey = action{
	name: "add-page-key", weight: 4, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil {
			return nil, errors.NotReady.With("no identity")
		}
		_, p := e.u.mutablePage(a)
		if p == nil || len(p.keys) >= maxPageKeys {
			return nil, errors.NotReady.With("no page with room for another key")
		}
		s := a.signer()
		k := newKey()
		ids, err := e.sign(ctx, s.url, func() txBuilder {
			return e.build(p).
				UpdateKeyPage().Add().Entry().Key(k, protocol.SignatureTypeED25519).FinishEntry().FinishOperation().
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
		if err != nil {
			return nil, err
		}
		e.u.mu.Lock()
		p.keys = append(p.keys, k)
		p.version++
		e.u.mu.Unlock()
		return ids, nil
	},
}

var updatePageKey = action{
	name: "update-page-key", weight: 3, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil {
			return nil, errors.NotReady.With("no identity")
		}
		_, p := e.u.mutablePage(a)
		if p == nil || len(p.keys) == 0 {
			return nil, errors.NotReady.With("no mutable page")
		}
		s := a.signer()
		i := e.u.intn(len(p.keys))
		k := newKey()
		ids, err := e.sign(ctx, s.url, func() txBuilder {
			return e.build(p).
				UpdateKeyPage().Update().
				Entry().Key(p.keys[i], protocol.SignatureTypeED25519).FinishEntry().
				To().Key(k, protocol.SignatureTypeED25519).FinishEntry().
				FinishOperation().
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
		if err != nil {
			return nil, err
		}
		e.u.mu.Lock()
		p.keys[i] = k
		p.version++
		e.u.mu.Unlock()
		return ids, nil
	},
}

var removePageKey = action{
	name: "remove-page-key", weight: 2, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil {
			return nil, errors.NotReady.With("no identity")
		}
		_, p := e.u.mutablePage(a)
		// Removing below the threshold would strand the page, and the last key
		// of a page cannot be removed at all.
		if p == nil || len(p.keys) <= 1 || uint64(len(p.keys)-1) < p.threshold {
			return nil, errors.NotReady.With("no page with a removable key")
		}
		s := a.signer()
		i := e.u.intn(len(p.keys))
		ids, err := e.sign(ctx, s.url, func() txBuilder {
			return e.build(p).
				UpdateKeyPage().Remove().Entry().Key(p.keys[i], protocol.SignatureTypeED25519).FinishEntry().FinishOperation().
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
		if err != nil {
			return nil, err
		}
		e.u.mu.Lock()
		p.keys = append(p.keys[:i], p.keys[i+1:]...)
		p.version++
		e.u.mu.Unlock()
		return ids, nil
	},
}

// --- authorities ----------------------------------------------------------

// updateAccountAuth flips an account's own authority between enabled and
// disabled. Adding a foreign authority would require that authority to
// countersign, which is a different (and much slower) shape of load.
var updateAccountAuth = action{
	name: "update-account-auth", weight: 2, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil || len(a.data) == 0 || a.signer() == nil {
			return nil, errors.NotReady.With("no data account")
		}
		s := a.signer()
		b := e.build(a.data[0]).UpdateAccountAuth()
		if e.u.intn(2) == 0 {
			b = b.Disable(a.books[0].url)
		} else {
			b = b.Enable(a.books[0].url)
		}
		return e.sign(ctx, s.url, func() txBuilder {
			return b.
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
	},
}

// --- tokens ---------------------------------------------------------------

// addToken builds a whole custom-token economy: issuer, holders, initial
// issuance. Detached because it is a multi-transaction sequence with ordering
// constraints, like the other growth actions.
var addToken = action{
	name: "add-token", weight: 1, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil || len(a.issuers) >= 3 {
			return nil, errors.NotReady.With("no identity wanting another token")
		}
		e.detach(ctx, "add-token", func(ctx context.Context) error { return e.growToken(ctx, a) })
		return nil, nil
	},
}

// issueTokens mints more of a custom token. Only the issuer's own authority
// can do this, which is a different authorization path from moving tokens.
var issueTokens = action{
	name: "issue-tokens", weight: 2, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a, tok := e.u.randIssuer(1)
		if a == nil || a.signer() == nil {
			return nil, errors.NotReady.With("no custom token")
		}
		s := a.signer()
		to := tok.accounts[e.u.intn(len(tok.accounts))]
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(tok.url).
				IssueTokens(1, tok.precision).To(to).
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
	},
}

// sendTokensCustom moves a non-ACME token. The resulting synthetic deposit
// carries a custom token URL rather than ACME, which ACME-only load never
// produces.
var sendTokensCustom = action{
	name: "send-tokens-custom", weight: 3, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a, tok := e.u.randIssuer(2)
		if a == nil || a.signer() == nil {
			return nil, errors.NotReady.With("no custom token with two holders")
		}
		s := a.signer()
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(tok.accounts[0]).
				SendTokens(1, tok.precision).To(tok.accounts[1]).
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
	},
}

// burnTokensCustom burns a non-ACME token, so the SyntheticBurnTokens routes
// to this issuer instead of to acc://ACME on the Directory.
var burnTokensCustom = action{
	name: "burn-tokens-custom", weight: 1, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a, tok := e.u.randIssuer(1)
		if a == nil || a.signer() == nil {
			return nil, errors.NotReady.With("no custom token")
		}
		s := a.signer()
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(tok.accounts[0]).
				BurnTokens(1, tok.precision).
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
	},
}

var lockAccount = action{
	name: "lock-account", weight: 1,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		// A lock is until a MAJOR block, and on a network that produces none
		// it never expires — the account is destroyed, not delayed (#4129).
		if !e.majorBlocksExist(ctx) {
			return nil, errors.NotReady.With("no major blocks yet — a lock would never expire")
		}
		l := e.u.randReadyLite()
		if l == nil {
			return nil, errors.NotReady.With("no confirmed lite account")
		}
		return e.sign(ctx, l.id, func() txBuilder {
			return e.build(l).
				LockAccount(1).
				SignWith(l.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(l.key)
		})
	},
}

// --- structural growth of existing identities -----------------------------
//
// Unlike growIdentity these are cheap enough to run inline: they are a single
// transaction plus a wait, and they broaden identities that already exist.

// These broaden identities that already exist. Like growIdentity they have to
// wait for the account to appear before recording it, so they run off the hot
// path — blocking here would drop the generation rate to whatever one round
// trip per block allows.

var addBook = action{
	name: "add-key-book", weight: 1, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil || len(a.books) >= 4 {
			return nil, errors.NotReady.With("no identity wanting another book")
		}
		e.detach(ctx, "add-key-book", func(ctx context.Context) error { return e.growBook(ctx, a) })
		return nil, nil
	},
}

var addAccount = action{
	name: "add-account", weight: 2, needsIdentity: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil || len(a.tokens)+len(a.data) >= 8 {
			return nil, errors.NotReady.With("no identity wanting another account")
		}
		e.detach(ctx, "add-account", func(ctx context.Context) error { return e.growAccount(ctx, a) })
		return nil, nil
	},
}
