// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"math/big"
	mrand "math/rand"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// universe is every account the generator knows about. It only ever grows, but
// the growth rate decays (see shouldGrow) so it stays small enough to keep in
// memory for the length of any realistic run.
type universe struct {
	mu  sync.Mutex
	rng *mrand.Rand

	lites []*liteAccount
	adis  []*identity
}

// liteAccount is a lite token account and the identity that signs for it.
type liteAccount struct {
	key  ed25519.PrivateKey
	acct *url.URL // .../ACME
	id   *url.URL // the lite identity, which is what signs and holds credits

	// ready means the account has been observed on chain with a credit
	// balance, so it can sign. A lite account the generator has merely sent
	// tokens to cannot: the deposit that creates it is asynchronous, and
	// signing needs credits on top of that.
	ready bool
}

// identity is an ADI and everything created under it.
type identity struct {
	url     *url.URL
	books   []*keyBook
	tokens  []*url.URL // ACME token accounts
	data    []*url.URL
	issuers []*tokenIssuer
}

// tokenIssuer is a custom token and the accounts that hold it.
//
// The accounts deliberately live under the SAME identity as the issuer.
// Creating a token account for a non-ACME issuer that is not local to the
// principal requires a TokenIssuerProof; keeping them together sidesteps that
// while still exercising every non-ACME path — issuance, transfer between
// accounts of a custom token, and burns that route to the issuer rather than
// to acc://ACME on the DN.
type tokenIssuer struct {
	url       *url.URL
	symbol    string
	precision uint64
	limited   bool       // a supply limit was set
	accounts  []*url.URL // token accounts holding this token
}

// keyBook is a key book and its pages. Page 1 is the generator's signer for
// everything under this identity; it is deliberately kept at threshold 1 and
// one key so it can always sign on its own. Later pages exist to carry a range
// of key counts and thresholds, and are mutated but never signed with — a page
// may be modified by any higher-priority (lower-indexed) page in its book.
type keyBook struct {
	url   *url.URL
	pages []*keyPage
}

type keyPage struct {
	url       *url.URL
	keys      []ed25519.PrivateKey
	threshold uint64
	version   uint64 // tracked locally; UpdateKeyPage bumps it
}

func newUniverse(rng *mrand.Rand) *universe {
	return &universe{rng: rng}
}

// shouldGrow reports whether this iteration should create new account
// structure. The probability starts at p0 and decays as the identity count
// grows, so the account set expands roughly like the square root of the number
// of transactions: fast enough to keep producing novel state forever, slow
// enough that a day-long run stays bounded.
func (u *universe) shouldGrow(p0 float64, scale int) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	// No special case for an empty universe: if the initial seed failed — a
	// node hiccup, a dropped message — the generator must keep trying, or the
	// entire identity half of the workload never runs for the whole session.
	p := p0 / (1 + float64(len(u.adis))/float64(scale))
	return u.rng.Float64() < p
}

func (u *universe) counts() (adis, books, pages, accounts, issuers int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	adis = len(u.adis)
	accounts = len(u.lites)
	for _, a := range u.adis {
		books += len(a.books)
		accounts += len(a.tokens) + len(a.data)
		issuers += len(a.issuers)
		for _, i := range a.issuers {
			accounts += len(i.accounts)
		}
		for _, b := range a.books {
			pages += len(b.pages)
		}
	}
	return
}

// randIssuer returns a random custom token that has at least minAccounts
// accounts, along with its identity. Returns nil if none qualify.
func (u *universe) randIssuer(minAccounts int) (*identity, *tokenIssuer) {
	u.mu.Lock()
	defer u.mu.Unlock()
	type pair struct {
		a *identity
		t *tokenIssuer
	}
	var cand []pair
	for _, a := range u.adis {
		for _, t := range a.issuers {
			if len(t.accounts) >= minAccounts {
				cand = append(cand, pair{a, t})
			}
		}
	}
	if len(cand) == 0 {
		return nil, nil
	}
	c := cand[u.rng.Intn(len(cand))]
	return c.a, c.t
}

func (u *universe) addIdentity(a *identity) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.adis = append(u.adis, a)
}

func (u *universe) addLite(l *liteAccount) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.lites = append(u.lites, l)
}

// randIdentity returns a random known identity, or nil if none are ready yet.
func (u *universe) randIdentity() *identity {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.adis) == 0 {
		return nil
	}
	return u.adis[u.rng.Intn(len(u.adis))]
}

// randLite returns a random known lite account, or nil if there are none. The
// result is safe to send to but not necessarily to sign with.
func (u *universe) randLite() *liteAccount {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.lites) == 0 {
		return nil
	}
	return u.lites[u.rng.Intn(len(u.lites))]
}

// randReadyLite returns a lite account known to exist and hold credits, so it
// can sign. Returns nil if none have been confirmed yet.
func (u *universe) randReadyLite() *liteAccount {
	u.mu.Lock()
	defer u.mu.Unlock()
	var ready []*liteAccount
	for _, l := range u.lites {
		if l.ready {
			ready = append(ready, l)
		}
	}
	if len(ready) == 0 {
		return nil
	}
	return ready[u.rng.Intn(len(ready))]
}

// unreadyLites returns the lite accounts still waiting to be confirmed.
func (u *universe) unreadyLites() []*liteAccount {
	u.mu.Lock()
	defer u.mu.Unlock()
	var out []*liteAccount
	for _, l := range u.lites {
		if !l.ready {
			out = append(out, l)
		}
	}
	return out
}

func (u *universe) markReady(l *liteAccount) {
	u.mu.Lock()
	defer u.mu.Unlock()
	l.ready = true
}

func (u *universe) intn(n int) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.rng.Intn(n)
}

// signer is page 1 of the identity's first book: threshold 1, one key, so it
// can always satisfy itself.
func (a *identity) signer() *keyPage {
	if len(a.books) == 0 || len(a.books[0].pages) == 0 {
		return nil
	}
	return a.books[0].pages[0]
}

func (a *identity) key() ed25519.PrivateKey {
	s := a.signer()
	if s == nil || len(s.keys) == 0 {
		return nil
	}
	return s.keys[0]
}

// mutablePage returns a page that is safe to modify: one in the identity's
// FIRST book, other than page 1.
//
// Both restrictions matter. A key page may only be modified by a
// higher-priority page in the SAME book, so a page in a secondary book cannot
// be reached by this identity's signer — the transaction would be accepted and
// then sit pending forever waiting on an authority that never signs. And page
// 1 is the signer itself, so changing its threshold or keys would strand the
// whole identity.
func (u *universe) mutablePage(a *identity) (*keyBook, *keyPage) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(a.books) == 0 || len(a.books[0].pages) < 2 {
		return nil, nil
	}
	b := a.books[0]
	p := b.pages[1+u.rng.Intn(len(b.pages)-1)]
	return b, p
}

// newLiteAccount generates a fresh lite token account. Note it does not route
// or otherwise care where the account lands.
func newLiteAccount(rng *mrand.Rand) *liteAccount {
	// crypto/rand for the same reason as adiName: account identity must be
	// unique per run even when the mix is replayed with a fixed seed.
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		panic(err)
	}
	key := ed25519.NewKeyFromSeed(seed)
	acct, err := protocol.LiteTokenAddress(key[32:], protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		panic(err)
	}
	return &liteAccount{key: key, acct: acct, id: acct.RootIdentity()}
}

func newKey() ed25519.PrivateKey {
	_, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return k
}

// adiName produces a unique ADI label. It draws from crypto/rand, NOT the
// seeded mix RNG: -seed governs which transactions are chosen, not which
// accounts exist. Deriving names from the seed would make two runs with the
// same seed collide on a network that still holds the first run's accounts,
// and the second run would then try to sign with keys that page never had.
//
// It is also deliberately random rather than searched for: the generator does
// not choose which partition an account lands on.
func adiName(prefix string) *url.URL {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return protocol.AccountUrl(fmt.Sprintf("%s-%x.acme", prefix, b))
}

// faucetAccount derives the genesis faucet key and ACME account from its seed,
// matching cmd/accumulated's createFaucet.
func faucetAccount(seedStr string) (ed25519.PrivateKey, *url.URL) {
	var seed storage.Key
	for _, s := range splitFields(seedStr) {
		seed = seed.Append(s)
	}
	sk := ed25519.NewKeyFromSeed(seed[:])
	u, err := protocol.LiteTokenAddress(sk[32:], protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		panic(err)
	}
	return sk, u
}

func splitFields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// acme returns n whole ACME.
func acme(n int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(n), big.NewInt(protocol.AcmePrecision))
}
