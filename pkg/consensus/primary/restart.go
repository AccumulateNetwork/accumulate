// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"crypto/ed25519"

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
	if h == nil || !ed25519.PublicKey(h.Author).Equal(p.config.KeyPair.Public()) {
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
