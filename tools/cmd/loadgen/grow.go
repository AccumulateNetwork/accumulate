// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"log"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Growing the account set is a sequence with ordering constraints — the ADI
// must exist before its page can be credited, the page must hold credits
// before it can sign, and only then can accounts be created under it. Running
// that inline would stall generation for the length of several blocks, so it
// runs in the background against a small pool of slots while the main loop
// keeps generating from the accounts that already exist.

// drainGrowth waits for in-flight account-creation sequences to finish, so a
// run does not abandon work it has already paid for.
func (e *env) drainGrowth(ctx context.Context, limit time.Duration) {
	deadline := time.Now().Add(limit)
	for len(e.growSlots) > 0 && time.Now().Before(deadline) && ctx.Err() == nil {
		time.Sleep(time.Second)
	}
}

// promoteLites confirms lite accounts on chain and gives them credits, so that
// actions which sign as a lite identity have somewhere valid to aim. It runs
// for the life of the generator.
func (e *env) promoteLites(ctx context.Context) {
	for ctx.Err() == nil {
		for _, l := range e.u.unreadyLites() {
			if ctx.Err() != nil {
				return
			}
			// Exists yet? The deposit that creates it is asynchronous.
			if _, err := e.Q.QueryAccount(ctx, l.acct, nil); err != nil {
				continue
			}
			// Signing costs credits, which a plain token deposit does not give.
			if !e.hasCredits(ctx, l.id) {
				_, _ = e.sign(ctx, e.treasury.id, func() txBuilder {
					return e.build(e.treasury).
						AddCredits().WithOracle(e.oracle).Purchase(liteCreditGrant).To(l.id).
						SignWith(e.treasury.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(e.treasury.key)
				})
				continue
			}
			e.u.markReady(l)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

// keepTreasuryFunded refills the treasury when it runs low. Without this a run
// silently degrades: once the ACME is gone every transfer, credit purchase and
// account creation fails, and the mix collapses to whatever costs nothing.
func (e *env) keepTreasuryFunded(ctx context.Context, useFaucetService bool) {
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		t := e.treasury
		e.treasuryCredits.Store(e.creditBalance(ctx, t.id))
		if useFaucetService && e.acmeBalance(ctx, t.acct) < minTreasuryAcme {
			for i := 0; i < 5; i++ {
				if _, err := e.c.Faucet(ctx, t.acct, api.FaucetOptions{Token: protocol.AcmeUrl()}); err != nil {
					break
				}
			}
			log.Printf("treasury topped up from the faucet")
		}

		if e.treasuryCredits.Load() < minTreasuryCreditUnits {
			if _, err := e.sign(ctx, t.id, func() txBuilder {
				return e.build(t).
					AddCredits().WithOracle(e.oracle).Purchase(treasuryCreditTopUp).To(t.id).
					SignWith(t.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(t.key)
			}); err == nil {
				log.Printf("treasury bought more credits")
			}
		}
	}
}

// detach runs an account-creation sequence off the generation loop, if a slot
// is free. Anything that has to wait for an account to appear belongs here:
// the loop must keep submitting at its configured rate.
func (e *env) detach(ctx context.Context, what string, fn func(context.Context) error) {
	select {
	case e.growSlots <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-e.growSlots }()
		if err := fn(ctx); err != nil && ctx.Err() == nil {
			log.Printf("%s: %v", what, err)
		}
	}()
}

// growAsync starts an account-creation sequence if a slot is free. When every
// slot is busy the growth opportunity is simply skipped; the decaying growth
// rate means missing one is immaterial.
func (e *env) growAsync(ctx context.Context) {
	select {
	case e.growSlots <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-e.growSlots }()
		if err := e.growIdentity(ctx); err != nil && ctx.Err() == nil {
			log.Printf("grow: %v", err)
		}
	}()
}

// growIdentity creates a whole identity: the ADI and its book, credits for its
// signer, a token account funded with ACME, a data account, and a second book
// carrying pages with a range of key counts and thresholds. The identity only
// becomes visible to the generator once it is usable.
func (e *env) growIdentity(ctx context.Context) error {
	signerKey := newKey()
	adiURL := adiName("lg")
	adi := &identity{url: adiURL}
	book := &keyBook{url: adiURL.JoinPath("book")}
	page1 := &keyPage{
		url:       adiURL.JoinPath("book", "1"),
		keys:      []ed25519.PrivateKey{signerKey},
		threshold: 1,
		version:   1,
	}
	book.pages = append(book.pages, page1)
	adi.books = append(adi.books, book)

	t := e.treasury
	ids, err := e.sign(ctx, t.id, func() txBuilder {
		return e.build(t).
			CreateIdentity(adiURL).WithKey(signerKey, protocol.SignatureTypeED25519).WithKeyBook(book.url).
			SignWith(t.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(t.key)
	})
	if err != nil {
		return errors.UnknownError.WithFormat("create identity: %w", err)
	}
	e.track.track("grow:create-identity", ids)
	if err := e.awaitAccount(ctx, page1.url, 3*time.Minute); err != nil {
		return err
	}

	// The signer needs credits before it can sign anything.
	ids, err = e.sign(ctx, t.id, func() txBuilder {
		return e.build(t).
			AddCredits().WithOracle(e.oracle).Purchase(pageCredits).To(page1.url).
			SignWith(t.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(t.key)
	})
	if err != nil {
		return errors.UnknownError.WithFormat("credit signer: %w", err)
	}
	e.track.track("grow:add-credits", ids)
	if err := e.awaitCredits(ctx, page1.url, 3*time.Minute); err != nil {
		return err
	}

	// A token account, funded so it can send.
	tokens := adiURL.JoinPath("tokens")
	ids, err = e.sign(ctx, page1.url, func() txBuilder {
		return e.build(adi).
			CreateTokenAccount(tokens).ForToken(protocol.AcmeUrl()).
			SignWith(page1.url).Version(page1.version).Timestamp(e.nonce.next()).PrivateKey(signerKey)
	})
	if err != nil {
		return errors.UnknownError.WithFormat("create token account: %w", err)
	}
	e.track.track("grow:create-token-account", ids)
	if err := e.awaitAccount(ctx, tokens, 3*time.Minute); err != nil {
		return err
	}
	adi.tokens = append(adi.tokens, tokens)

	if _, err := e.sign(ctx, t.id, func() txBuilder {
		return e.build(t).
			SendTokens(fundTokenAccount, protocol.AcmePrecisionPower).To(tokens).
			SignWith(t.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(t.key)
	}); err != nil {
		return errors.UnknownError.WithFormat("fund token account: %w", err)
	}

	// A data account.
	data := adiURL.JoinPath("data")
	ids, err = e.sign(ctx, page1.url, func() txBuilder {
		return e.build(adi).
			CreateDataAccount(data).
			SignWith(page1.url).Version(page1.version).Timestamp(e.nonce.next()).PrivateKey(signerKey)
	})
	if err != nil {
		return errors.UnknownError.WithFormat("create data account: %w", err)
	}
	e.track.track("grow:create-data-account", ids)
	if err := e.awaitAccount(ctx, data, 3*time.Minute); err != nil {
		return err
	}
	adi.data = append(adi.data, data)

	// The identity is usable now; publish it before the optional extras so the
	// generator can start using it immediately.
	e.u.addIdentity(adi)

	// A second page in the first book, with a range of keys and a threshold
	// somewhere within that range. This is state variety for its own sake: the
	// page is mutated later but never used as a signer.
	if err := e.growPage(ctx, adi, book); err != nil {
		return err
	}
	return nil
}

// growPage adds a key page to a book with a random key count and a threshold
// somewhere in 1..keys, so the network accumulates pages across the whole
// range of shapes rather than a single canonical one.
func (e *env) growPage(ctx context.Context, adi *identity, book *keyBook) error {
	signer := adi.signer()
	if signer == nil {
		return errors.NotReady.With("identity has no signer")
	}

	n := 1 + e.u.intn(maxPageKeys)
	keys := make([]ed25519.PrivateKey, n)
	// CreateKeyPage's principal is the BOOK the page is added to, not the ADI.
	b := e.build(book.url).CreateKeyPage()
	for i := range keys {
		keys[i] = newKey()
		b = b.WithEntry().Key(keys[i], protocol.SignatureTypeED25519).FinishEntry()
	}

	ids, err := e.sign(ctx, signer.url, func() txBuilder {
		return b.
			SignWith(signer.url).Version(signer.version).Timestamp(e.nonce.next()).PrivateKey(adi.key())
	})
	if err != nil {
		return errors.UnknownError.WithFormat("create key page: %w", err)
	}
	e.track.track("grow:create-key-page", ids)

	page := &keyPage{
		url:       protocol.FormatKeyPageUrl(book.url, uint64(len(book.pages))),
		keys:      keys,
		threshold: 1,
		version:   1,
	}
	if err := e.awaitAccount(ctx, page.url, 3*time.Minute); err != nil {
		return err
	}

	e.u.mu.Lock()
	book.pages = append(book.pages, page)
	e.u.mu.Unlock()

	// Pages need credits of their own to be updated later.
	if _, err := e.sign(ctx, e.treasury.id, func() txBuilder {
		return e.build(e.treasury).
			AddCredits().WithOracle(e.oracle).Purchase(pageCredits).To(page.url).
			SignWith(e.treasury.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(e.treasury.key)
	}); err != nil {
		return errors.UnknownError.WithFormat("credit page: %w", err)
	}

	// Give it a threshold somewhere in range. Signing is done by page 1, which
	// outranks this page, so a high threshold here never strands anything.
	if n > 1 {
		threshold := uint64(1 + e.u.intn(n))
		ids, err := e.sign(ctx, signer.url, func() txBuilder {
			return e.build(page).
				UpdateKeyPage().SetThreshold(threshold).
				SignWith(signer.url).Version(signer.version).Timestamp(e.nonce.next()).PrivateKey(adi.key())
		})
		if err != nil {
			return errors.UnknownError.WithFormat("set threshold: %w", err)
		}
		e.track.track("grow:set-threshold", ids)
		e.u.mu.Lock()
		page.threshold = threshold
		page.version++
		e.u.mu.Unlock()
	}
	return nil
}

// growToken creates a custom token and everything needed to actually use it:
// the issuer, two accounts that hold it, and an initial issuance.
//
// Creating the issuer alone would be a dangling transaction — it succeeds,
// reports no rejection, and leaves nothing any later action can reference. The
// point of a custom token is the paths it opens up: issuance, a SendTokens
// carrying a non-ACME token URL, and a burn that routes to this issuer instead
// of to acc://ACME on the Directory.
func (e *env) growToken(ctx context.Context, adi *identity) error {
	signer := adi.signer()
	if signer == nil {
		return errors.NotReady.With("identity has no signer")
	}
	// The most expensive transaction there is. A signer that cannot pay has
	// its signature rejected and the transaction stranded, so skip instead.
	if e.creditBalance(ctx, signer.url) < uint64(protocol.FeeCreateToken)+protocol.CreditPrecision {
		return errors.NotReady.With("signer cannot afford to create a token")
	}

	n := e.u.intn(1 << 20)
	tok := &tokenIssuer{
		url:       adi.url.JoinPath("tok" + itoa(n)),
		symbol:    "LG" + itoa(n%1000),
		precision: uint64(e.u.intn(maxTokenPrecision + 1)),
	}

	b := e.build(adi).CreateToken(tok.url).WithSymbol(tok.symbol).WithPrecision(tok.precision)
	// Give some tokens a supply limit so that path is exercised too. It is set
	// far above what the generator issues, so it constrains nothing.
	if e.u.intn(2) == 0 {
		tok.limited = true
		b = b.WithSupplyLimit(tokenSupplyLimit)
	}
	ids, err := e.sign(ctx, signer.url, func() txBuilder {
		return b.
			SignWith(signer.url).Version(signer.version).Timestamp(e.nonce.next()).PrivateKey(adi.key())
	})
	if err != nil {
		return errors.UnknownError.WithFormat("create token: %w", err)
	}
	e.track.track("grow:create-token", ids)
	if err := e.awaitAccount(ctx, tok.url, 3*time.Minute); err != nil {
		return err
	}

	// Two accounts, so tokens have somewhere to move between. They are local
	// to the issuer, so no TokenIssuerProof is needed.
	for i := 0; i < 2; i++ {
		acct := adi.url.JoinPath("tok" + itoa(n) + "-" + itoa(i))
		ids, err := e.sign(ctx, signer.url, func() txBuilder {
			return e.build(adi).
				CreateTokenAccount(acct).ForToken(tok.url).
				SignWith(signer.url).Version(signer.version).Timestamp(e.nonce.next()).PrivateKey(adi.key())
		})
		if err != nil {
			return errors.UnknownError.WithFormat("create token account: %w", err)
		}
		e.track.track("grow:create-token-holder", ids)
		if err := e.awaitAccount(ctx, acct, 3*time.Minute); err != nil {
			return err
		}
		tok.accounts = append(tok.accounts, acct)
	}

	// Issue an initial supply into the first account.
	ids, err = e.sign(ctx, signer.url, func() txBuilder {
		return e.build(tok.url).
			IssueTokens(initialIssuance, tok.precision).To(tok.accounts[0]).
			SignWith(signer.url).Version(signer.version).Timestamp(e.nonce.next()).PrivateKey(adi.key())
	})
	if err != nil {
		return errors.UnknownError.WithFormat("issue tokens: %w", err)
	}
	e.track.track("grow:issue-tokens", ids)

	e.u.mu.Lock()
	adi.issuers = append(adi.issuers, tok)
	e.u.mu.Unlock()
	return nil
}

// growBook adds another key book to an existing identity, with one page.
func (e *env) growBook(ctx context.Context, adi *identity) error {
	signer := adi.signer()
	if signer == nil {
		return errors.NotReady.With("identity has no signer")
	}

	key := newKey()
	bookURL := adi.url.JoinPath("book" + itoa(len(adi.books)+1))
	ids, err := e.sign(ctx, signer.url, func() txBuilder {
		return e.build(adi).
			CreateKeyBook(bookURL).WithKey(key, protocol.SignatureTypeED25519).
			SignWith(signer.url).Version(signer.version).Timestamp(e.nonce.next()).PrivateKey(adi.key())
	})
	if err != nil {
		return errors.UnknownError.WithFormat("create key book: %w", err)
	}
	e.track.track("grow:create-key-book", ids)

	if err := e.awaitAccount(ctx, bookURL, 3*time.Minute); err != nil {
		return err
	}
	e.u.mu.Lock()
	adi.books = append(adi.books, &keyBook{
		url: bookURL,
		pages: []*keyPage{{
			url:       protocol.FormatKeyPageUrl(bookURL, 0),
			keys:      []ed25519.PrivateKey{key},
			threshold: 1,
			version:   1,
		}},
	})
	e.u.mu.Unlock()
	return nil
}

// growAccount adds another token or data account to an existing identity.
func (e *env) growAccount(ctx context.Context, adi *identity) error {
	signer := adi.signer()
	if signer == nil {
		return errors.NotReady.With("identity has no signer")
	}

	if e.u.intn(2) == 0 {
		u := adi.url.JoinPath("tokens" + itoa(len(adi.tokens)+1))
		ids, err := e.sign(ctx, signer.url, func() txBuilder {
			return e.build(adi).
				CreateTokenAccount(u).ForToken(protocol.AcmeUrl()).
				SignWith(signer.url).Version(signer.version).Timestamp(e.nonce.next()).PrivateKey(adi.key())
		})
		if err != nil {
			return errors.UnknownError.WithFormat("create token account: %w", err)
		}
		e.track.track("grow:create-token-account", ids)
		if err := e.awaitAccount(ctx, u, 3*time.Minute); err != nil {
			return err
		}
		e.u.mu.Lock()
		adi.tokens = append(adi.tokens, u)
		e.u.mu.Unlock()
		return nil
	}

	u := adi.url.JoinPath("data" + itoa(len(adi.data)+1))
	ids, err := e.sign(ctx, signer.url, func() txBuilder {
		return e.build(adi).
			CreateDataAccount(u).
			SignWith(signer.url).Version(signer.version).Timestamp(e.nonce.next()).PrivateKey(adi.key())
	})
	if err != nil {
		return errors.UnknownError.WithFormat("create data account: %w", err)
	}
	e.track.track("grow:create-data-account", ids)
	if err := e.awaitAccount(ctx, u, 3*time.Minute); err != nil {
		return err
	}
	e.u.mu.Lock()
	adi.data = append(adi.data, u)
	e.u.mu.Unlock()
	return nil
}

// build starts a transaction whose principal is the given thing. It exists so
// actions read as one line each.
func (e *env) build(principal any) build.TransactionBuilder {
	switch p := principal.(type) {
	case *liteAccount:
		return build.Transaction().For(p.acct)
	case *identity:
		return build.Transaction().For(p.url)
	case *keyPage:
		return build.Transaction().For(p.url)
	case *url.URL:
		return build.Transaction().For(p)
	default:
		panic("unsupported principal")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

const (
	// Credit amounts come in two units and mixing them is an easy, silent
	// mistake: the protocol's fees and an account's CreditBalance are in
	// credit-UNITS (protocol.CreditPrecision units per credit), while
	// AddCredits().Purchase() takes whole CREDITS. Constants below say which.
	//
	// Funding is finite: the faucet grants a fixed amount per call and a long
	// run makes hundreds of thousands of transactions. Transfers therefore
	// move dust — the point is to exercise the transaction, not move value.
	sendAmount = 0.0001 // ACME per transfer or burn
	liteSeed   = 0.1    // ACME the treasury seeds a lite with, enough to relay many times

	liteCreditGrant = 500   // credits: a lite identity only signs cheap things
	pageCreditGrant = 5000  // credits: treasury per top-up for a key page
	adiCreditGrant  = 500   // credits: an identity buys this for a page from its OWN ACME
	pageCredits     = 30000 // credits: what a new key page starts with

	// A page must be able to afford the expensive transactions, not just the
	// common ones — creating a token issuer alone costs 5,000 credits. A page
	// that cannot pay has its SIGNATURE rejected, which leaves the transaction
	// pending forever with no signature recorded rather than failing loudly.

	fundTokenAccount = 1000 // ACME into each new token account: enough that the
	//                        identity can send, burn, and buy its own credits for
	//                        a long run, so cross-partition load originates from
	//                        every identity's partition, not just the treasury's.
	maxPageKeys      = 5 // most keys a generated page may carry

	// Custom tokens.
	maxTokenPrecision = 4         // issuers span precision 0..4
	initialIssuance   = 1_000_000 // whole tokens minted into the first holder
	tokenSupplyLimit  = 1_000_000_000

	// Treasury refill thresholds.
	minTreasuryAcme        = 50        // whole ACME
	minTreasuryCreditUnits = 5_000_000 // credit-units (= 50,000 credits)
	// treasuryFloorUnits is the balance below which treasury-signed actions
	// skip rather than risk being stranded between refills.
	treasuryFloorUnits = 500_000 // credit-units (= 5,000 credits)
	// estimatedFeeUnits is debited from the cached treasury balance on every
	// treasury-signed submission. It deliberately over-estimates the common
	// fees: erring high just means refilling sooner.
	estimatedFeeUnits   = 5_000  // credit-units (= 50 credits)
	treasuryCreditTopUp = 50_000 // credits to buy when below
)
