// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"log/slog"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// OnCertificateReceived handles an incoming certificate from another validator.
func (p *Primary) OnCertificateReceived(cert *types.Certificate) {
	if cert == nil {
		return
	}

	// Verify certificate
	if err := cert.Verify(p.committee); err != nil {
		slog.Debug("Invalid certificate",
			"error", err,
			"digest", cert.Digest().String())
		return
	}

	// Insert into DAG (validates parents exist)
	var err error
	if cert.Round() == 0 {
		err = p.dag.InsertGenesis(cert)
	} else {
		err = p.dag.Insert(cert)
	}
	if err != nil {
		slog.Debug("Failed to insert certificate into DAG",
			"error", err,
			"digest", cert.Digest().String())
		return
	}

	slog.Debug("Inserted certificate from peer",
		"digest", cert.Digest().String(),
		"round", cert.Round(),
		"author", hexEncode(cert.Author()))

	// Signal for Bullshark
	p.signalNewCertificate(cert)

	// Maybe we can advance round now
	p.tryAdvanceRound()
}

// tryAdvanceRound attempts to advance to the next round if we have enough certificates.
func (p *Primary) tryAdvanceRound() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Can advance when we have 2f+1 certificates in current round
	if !p.dag.HasQuorum(p.currentRound, p.committee) {
		return
	}

	// Rate limit: don't advance faster than MinRoundInterval
	now := time.Now()
	if !p.lastRoundAdvance.IsZero() {
		elapsed := now.Sub(p.lastRoundAdvance)
		if elapsed < p.config.MinRoundInterval {
			// Too soon, wait for the ticker to try again
			return
		}
	}

	oldRound := p.currentRound
	p.currentRound++
	p.lastRoundAdvance = now

	slog.Info("Advanced to new round",
		"oldRound", oldRound,
		"newRound", p.currentRound)

	// Clean up old headers
	go p.cleanupOldHeaders()

	// Create header for new round (outside lock to avoid deadlock)
	go p.tryCreateAndBroadcastHeader()
}

// AdvanceRound forcibly advances to the next round.
// This is useful for testing or when manual round advancement is needed.
func (p *Primary) AdvanceRound() {
	p.mu.Lock()
	oldRound := p.currentRound
	p.currentRound++
	p.mu.Unlock()

	slog.Info("Forcibly advanced to new round",
		"oldRound", oldRound,
		"newRound", p.currentRound)

	p.tryCreateAndBroadcastHeader()
}

// SetRound sets the current round (useful for testing or sync).
func (p *Primary) SetRound(round types.Round) {
	p.mu.Lock()
	p.currentRound = round
	p.mu.Unlock()
}

// SetEpoch sets the current epoch (useful for committee changes).
func (p *Primary) SetEpoch(epoch uint64) {
	p.mu.Lock()
	p.currentEpoch = epoch
	p.mu.Unlock()
}

// UpdateCommittee updates the committee (for epoch transitions).
func (p *Primary) UpdateCommittee(committee *types.Committee) {
	p.mu.Lock()
	p.committee = committee
	p.currentEpoch = committee.Epoch
	p.mu.Unlock()

	slog.Info("Updated committee",
		"epoch", committee.Epoch,
		"validators", len(committee.Validators))
}

// GetOurCertificate returns our certificate for the given round, if any.
func (p *Primary) GetOurCertificate(round types.Round) *types.Certificate {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.ourCerts[round]
}

// HasCertificateForRound returns true if we have created a certificate for the round.
func (p *Primary) HasCertificateForRound(round types.Round) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, ok := p.ourCerts[round]
	return ok
}

// PendingVoteCount returns the number of pending votes for a header.
func (p *Primary) PendingVoteCount(headerDigest types.HeaderDigest) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.pendingVotes[headerDigest])
}

// OurHeadersCount returns the number of headers we're collecting votes for.
func (p *Primary) OurHeadersCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.ourHeaders)
}
