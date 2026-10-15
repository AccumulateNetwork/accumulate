// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"crypto/ed25519"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// SetOnAuthored installs the hook that persists each header this primary
// authors, before it is broadcast (#4448). Call before Start.
func (p *Primary) SetOnAuthored(fn func(*types.Header) error) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	p.onAuthored = fn
}

// RestoreAuthored tells a restarted primary the last header it authored
// before it stopped (#4448). It never authors that round, or any below it,
// again (#4159 stall 3). If that header's certificate is not in the DAG and
// the round is still current, the header goes back into vote collection, so
// the ordinary rebroadcast sends the same header again; the primary's round
// is raised to it, since the node had reached it. Call before Start, after
// the DAG has been restored.
func (p *Primary) RestoreAuthored(h *types.Header) {
	if h == nil || !h.Author.Equal(p.config.KeyPair.Public()) {
		return
	}
	p.roundMu.Lock()
	if h.Round > p.currentRound {
		p.currentRound = h.Round
	}
	current := p.currentRound
	p.roundMu.Unlock()

	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	if !p.hasAuthored || h.Round > p.lastAuthoredRound {
		p.hasAuthored = true
		p.lastAuthoredRound = h.Round
	}
	digest := h.Digest()
	if h.Round+1 < current || p.dag.Contains(types.CertificateDigest(digest)) {
		return
	}
	p.ourHeaders[digest] = h
	vote := types.NewVote(digest, h.Round, h.Epoch, p.config.KeyPair.Public().(ed25519.PublicKey))
	if err := vote.Sign(p.config.KeyPair); err == nil {
		p.pendingVotes[digest] = []*types.Vote{vote}
	} else {
		p.pendingVotes[digest] = nil
	}
}

// LastAuthoredRound is the highest round this primary has authored, and
// whether it has authored any.
func (p *Primary) LastAuthoredRound() (types.Round, bool) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	return p.lastAuthoredRound, p.hasAuthored
}

// RoundStatus is what a primary holds for its current round: what a stalled
// partition's log line names so "will not author", "cannot collect votes" and
// "certificates never formed" can be told apart (run 20261002T090546Z: BVN3's
// four validators restored the same round and none left it).
type RoundStatus struct {
	Round        types.Round
	LastAuthored types.Round
	HasAuthored  bool
	OwnHeader    bool // this primary holds its header for the round, awaiting votes
	OwnVotes     int  // votes collected for it
	CertsHere    int  // certificates in the DAG for the round
	CertsBefore  int  // and for the round before it

	// Authors is how many of the committee this primary has heard producing
	// at the round in the last HeardWithin: a header or a vote for it, and
	// itself. Committee and Quorum are the committee's size and
	// the count a round needs. Authors below Quorum is a partition that has
	// lost its quorum, not a round that is merely slow (#4512).
	Authors, Committee, Quorum int
}

// RoundStatus reports the primary's current round as it holds it.
func (p *Primary) RoundStatus() RoundStatus {
	p.roundMu.Lock()
	r := p.currentRound
	p.roundMu.Unlock()
	st := RoundStatus{Round: r, CertsHere: p.dag.RoundCount(r)}
	if r > 0 {
		st.CertsBefore = p.dag.RoundCount(r - 1)
	}
	authors := map[string]bool{string(p.config.KeyPair.Public().(ed25519.PublicKey)): true}
	p.pendingMu.Lock()
	st.LastAuthored, st.HasAuthored = p.lastAuthoredRound, p.hasAuthored
	for d, h := range p.ourHeaders {
		if h.Round == r {
			st.OwnHeader = true
			st.OwnVotes = len(p.pendingVotes[d])
		}
	}
	p.pendingMu.Unlock()
	p.seenMu.Lock()
	now := time.Now()
	for a, seen := range p.headerSeen {
		if seen.round >= r && now.Sub(seen.at) <= HeardWithin {
			authors[a] = true
		}
	}
	p.seenMu.Unlock()
	st.Authors = len(authors)
	p.committeeMu.RLock()
	st.Committee, st.Quorum = p.committee.Len(), p.committee.QuorumCount()
	p.committeeMu.RUnlock()
	return st
}

// HeardWithin is how recently a committee member must have been heard at the
// current round to count as producing there (RoundStatus.Authors). A live
// member in a round that does not complete keeps sending: it rebroadcasts its
// header about once a second, its certified one too once the round has
// stalled (#4581), and re-sends its vote on each rebroadcast it sees. A stopped one's last header stays in the DAG and in memory, and
// counting it made a partition that lost two of four report three producing.
const HeardWithin = 10 * time.Second

type seenAt struct {
	round types.Round
	at    time.Time
}

// noteHeaderSeen records that a committee member sent a header or a vote for
// round, now (RoundStatus.Authors).
func (p *Primary) noteHeaderSeen(author []byte, round types.Round) {
	p.seenMu.Lock()
	defer p.seenMu.Unlock()
	if p.headerSeen == nil {
		p.headerSeen = map[string]seenAt{}
	}
	cur := p.headerSeen[string(author)]
	if round >= cur.round {
		p.headerSeen[string(author)] = seenAt{round: round, at: time.Now()}
	}
}
