// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"bytes"
	"crypto/ed25519"
	"log/slog"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// OnVoteReceived handles an incoming vote for our header.
func (p *Primary) OnVoteReceived(vote *types.Vote) {
	if vote == nil {
		return
	}

	p.votesReceived.Add(1)

	// CHEAP CHECK FIRST: Committee membership (~21ns)
	// This prevents CPU exhaustion from non-validator vote spam
	p.committeeMu.RLock()
	inCommittee := p.committee.ContainsValidator(vote.Author)
	quorumCount := p.committee.QuorumCount()
	p.committeeMu.RUnlock()

	if !inCommittee {
		slog.Info("Vote from unknown validator",
			"author", hexEncode(vote.Author))
		return
	}

	// EXPENSIVE CHECK SECOND: Signature verification (~29µs)
	// Only verify signatures from committee members
	if err := vote.Verify(); err != nil {
		slog.Info("Invalid vote signature",
			"error", err,
			"author", hexEncode(vote.Author))
		return
	}

	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()

	// Check vote is for a header we created
	header, ok := p.ourHeaders[vote.HeaderDigest]
	if !ok {
		slog.Debug("Vote for unknown header",
			"headerDigest", vote.HeaderDigest.String())
		return
	}

	// Check vote round/epoch matches
	if vote.Round != header.Round {
		slog.Info("Vote round mismatch",
			"voteRound", vote.Round,
			"headerRound", header.Round)
		return
	}

	if vote.Epoch != header.Epoch {
		slog.Info("Vote epoch mismatch",
			"voteEpoch", vote.Epoch,
			"headerEpoch", header.Epoch)
		return
	}

	// Check if we've already reached the vote limit (spam protection)

	// Get current votes for this header
	votes := p.pendingVotes[vote.HeaderDigest]

	// Check for duplicates FIRST (before counting against limit)
	// This prevents spam attack where one validator sends many duplicate votes
	// to fill the vote limit and block legitimate votes from other validators
	for _, v := range votes {
		if bytes.Equal(v.Author, vote.Author) {
			slog.Debug("Duplicate vote from author",
				"headerDigest", vote.HeaderDigest.String(),
				"author", hexEncode(vote.Author))
			return // already have vote from this author
		}
	}

	// Now check vote limit (only counting unique votes)
	// Maximum votes = 2x the quorum threshold (2f+1)
	// This allows some safety margin while preventing spam attacks
	maxVotes := quorumCount * VotesPerHeaderMultiplier
	if len(votes) >= maxVotes {
		slog.Warn("Vote limit reached for header - potential spam attack",
			"headerDigest", vote.HeaderDigest.String(),
			"currentVotes", len(votes),
			"maxVotes", maxVotes,
			"quorumCount", quorumCount,
			"author", hexEncode(vote.Author))
		return
	}

	// Add the unique vote
	p.pendingVotes[vote.HeaderDigest] = append(votes, vote)

	slog.Info("Added vote",
		"headerDigest", vote.HeaderDigest.String(),
		"author", hexEncode(vote.Author),
		"totalVotes", len(p.pendingVotes[vote.HeaderDigest]))

	// Try to create certificate
	p.tryCreateCertificateLocked(vote.HeaderDigest)
}

// tryCreateCertificateLocked attempts to create a certificate from collected votes.
// Must be called with p.pendingMu held.
func (p *Primary) tryCreateCertificateLocked(headerDigest types.HeaderDigest) {
	votes := p.pendingVotes[headerDigest]
	header := p.ourHeaders[headerDigest]

	if header == nil {
		return
	}

	// Calculate total stake from votes (needs committeeMu for reading stake)
	p.committeeMu.RLock()
	var totalStake uint64
	for _, v := range votes {
		totalStake += p.committee.StakeOf(v.Author)
	}
	hasQuorum := p.committee.HasQuorum(totalStake)
	quorumThreshold := p.committee.QuorumThreshold()
	p.committeeMu.RUnlock()

	// Need 2f+1 stake
	if !hasQuorum {
		slog.Info("Not enough stake for certificate",
			"headerDigest", headerDigest.String(),
			"totalStake", totalStake,
			"threshold", quorumThreshold)
		return
	}

	// Create certificate (needs committeeMu for finding validators)
	cert := p.createCertificateFromVotes(header, votes)
	if cert == nil {
		return
	}

	// Store in our certs
	p.ourCerts[cert.Round()] = cert
	p.certificatesCreated.Add(1)

	slog.Info("Created certificate",
		"partition", p.config.Partition,
		"digest", cert.Digest().String(),
		"round", cert.Round(),
		"signers", len(cert.SignedAuthorities))

	// Clean up pending state for this header
	delete(p.pendingVotes, headerDigest)
	delete(p.ourHeaders, headerDigest)

	// Insert into DAG and broadcast (outside lock)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		// Check if context is cancelled
		if p.ctx != nil {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
		}
		// Insert into DAG
		var err error
		if cert.Round() == 0 {
			err = p.dag.InsertGenesis(cert)
		} else {
			err = p.dag.Insert(cert)
		}
		if err != nil {
			slog.Warn("Failed to insert certificate into DAG",
				"error", err,
				"digest", cert.Digest().String())
			return
		}

		// Broadcast
		if p.gossip != nil {
			if err := p.gossip.BroadcastCertificate(p.ctx, cert); err != nil {
				slog.Warn("Failed to broadcast certificate",
					"error", err,
					"digest", cert.Digest().String())
			}
		}

		// Signal for Bullshark
		p.signalNewCertificate(cert)
	}()
}

// createCertificateFromVotes creates a certificate from the header and collected votes.
// Uses committeeMu internally for finding validators.
func (p *Primary) createCertificateFromVotes(header *types.Header, votes []*types.Vote) *types.Certificate {
	if len(votes) == 0 {
		return nil
	}

	sigs := make([][]byte, len(votes))
	authors := make([]uint16, len(votes))

	p.committeeMu.RLock()
	for i, v := range votes {
		sigs[i] = make([]byte, len(v.Signature))
		copy(sigs[i], v.Signature)

		idx, found := p.committee.FindValidator(v.Author)
		if !found {
			// This shouldn't happen as we check in OnVoteReceived
			continue
		}
		authors[i] = idx
	}
	p.committeeMu.RUnlock()

	return types.NewCertificate(header, sigs, authors)
}

// OnHeaderReceived handles an incoming header from another validator.
// If valid, we vote on it.
func (p *Primary) OnHeaderReceived(header *types.Header) {
	if header == nil {
		return
	}

	slog.Info("Header handled by primary",
		"partition", p.config.Partition,
		"author", hexEncode(header.Author),
		"round", header.Round)

	// Verify header signature
	if err := header.Verify(); err != nil {
		slog.Info("Invalid header signature",
			"error", err,
			"author", hexEncode(header.Author))
		return
	}

	// Check author is in committee (uses committeeMu)
	p.committeeMu.RLock()
	inCommittee := p.committee.ContainsValidator(header.Author)
	p.committeeMu.RUnlock()

	if !inCommittee {
		slog.Info("Header from unknown validator",
			"author", hexEncode(header.Author))
		return
	}

	// Don't vote on our own headers
	pubKey := p.config.KeyPair.Public().(ed25519.PublicKey)
	if bytes.Equal(header.Author, pubKey) {
		return
	}

	// Check epoch and round (uses roundMu)
	p.roundMu.Lock()
	currentEpoch := p.currentEpoch
	currentRound := p.currentRound
	p.roundMu.Unlock()

	// Check epoch matches
	if header.Epoch != currentEpoch {
		slog.Info("Header epoch mismatch",
			"headerEpoch", header.Epoch,
			"currentEpoch", currentEpoch)
		return
	}

	// Check round is acceptable (not too old, not too far ahead)
	minRound := currentRound
	if minRound > 1 {
		minRound = currentRound - 1
	} else {
		minRound = 0
	}
	maxRound := currentRound + 1

	if header.Round < minRound || header.Round > maxRound {
		slog.Info("Header round out of range",
			"headerRound", header.Round,
			"currentRound", currentRound,
			"minRound", minRound,
			"maxRound", maxRound)

		// A round mismatch after an outage is a deadlock without recovery:
		// certificates are broadcast exactly once, so a node that missed
		// them can neither advance (it rejects newer headers) nor help a
		// stale author advance (the author never learns its round already
		// completed). Sync rounds in both directions (#4057).
		switch {
		case header.Round > maxRound:
			// We are behind — pull the certificates that will advance us
			p.requestRoundCatchUp(currentRound, header.Round)
		case header.Round < minRound:
			// The author is behind — push the certificates it is missing
			p.pushCertsForStaleRound(header.Round, currentRound)
		}
		return
	}

	// Check if we already voted on this header (uses pendingMu)
	headerDigest := header.Digest()
	p.pendingMu.Lock()
	if _, voted := p.votedHeaders[headerDigest]; voted {
		// The author rebroadcasts a header until it achieves quorum. If we
		// see the header again, our vote may have been lost — votes are
		// otherwise sent exactly once, which permanently stalls the round if
		// the gossip mesh was still forming when we voted (#4054). Resend the
		// stored vote; receivers deduplicate.
		vote := p.sentVotes[headerDigest]
		p.pendingMu.Unlock()
		if vote != nil {
			p.broadcastVoteAsync(vote, headerDigest)
		}
		return
	}
	p.pendingMu.Unlock()

	// Check we have all parent certificates
	for _, parentDigest := range header.Parents {
		if p.dag.GetByDigest(parentDigest) == nil {
			slog.Info("Missing parent for header",
				"headerDigest", headerDigest.String(),
				"parentDigest", parentDigest.String())
			return // missing parent, can't vote
		}
	}

	// Create and send vote
	vote := types.NewVote(headerDigest, header.Round, header.Epoch, pubKey)
	if err := vote.Sign(p.config.KeyPair); err != nil {
		slog.Warn("Failed to sign vote",
			"error", err)
		return
	}

	// Mark as voted (store round for cleanup) and keep the vote so it can be
	// resent if the author rebroadcasts the header (#4054)
	p.pendingMu.Lock()
	p.votedHeaders[headerDigest] = header.Round
	p.sentVotes[headerDigest] = vote
	p.pendingMu.Unlock()

	p.votesSent.Add(1)

	slog.Info("Voting on header",
		"partition", p.config.Partition,
		"headerDigest", headerDigest.String(),
		"author", hexEncode(header.Author),
		"round", header.Round)

	// Broadcast vote
	p.broadcastVoteAsync(vote, headerDigest)
}

// broadcastVoteAsync broadcasts a vote in the background.
func (p *Primary) broadcastVoteAsync(vote *types.Vote, headerDigest types.HeaderDigest) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		// Check if context is cancelled
		if p.ctx != nil {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
		}
		if p.gossip == nil {
			return
		}
		if err := p.gossip.BroadcastVote(p.ctx, vote); err != nil {
			slog.Warn("Failed to broadcast vote",
				"error", err,
				"headerDigest", headerDigest.String())
		}
	}()
}

// hexEncode returns a short hex representation of a public key.
func hexEncode(key ed25519.PublicKey) string {
	if len(key) < 4 {
		return ""
	}
	return types.HeaderDigest(key).String()[:8]
}

// requestRoundCatchUp pulls the certificates of rounds (current, target] so
// this node can advance after falling behind (#4057). Paced to once per
// second; the certificate handler inserts what arrives and round advancement
// follows naturally.
func (p *Primary) requestRoundCatchUp(current, target types.Round) {
	if p.certSyncer == nil {
		return
	}

	p.roundSyncMu.Lock()
	if time.Since(p.lastRoundPull) < time.Second {
		p.roundSyncMu.Unlock()
		return
	}
	p.lastRoundPull = time.Now()
	p.roundSyncMu.Unlock()

	// Request at most MaxSyncRounds rounds, starting from where we are —
	// certificates insert parent-first, so pulling the oldest gap first
	// makes steady forward progress even across a large gap.
	first := current
	if first > 0 {
		first-- // Re-fetch the previous round in case our copy is partial
	}
	var rounds []types.Round
	for r := first; r <= target && len(rounds) < gossip.MaxSyncRounds; r++ {
		rounds = append(rounds, r)
	}
	p.certSyncer.RequestRounds(rounds)
}

// pushCertsForStaleRound rebroadcasts the certificates of a stale round (and
// the following round) when a peer is observed rebroadcasting a header for
// it (#4057). The peer is behind: it never saw these certificates — they are
// broadcast exactly once — so it can neither complete its round nor advance.
// Paced to once per second.
func (p *Primary) pushCertsForStaleRound(stale, current types.Round) {
	if p.gossip == nil {
		return
	}

	p.roundSyncMu.Lock()
	if time.Since(p.lastRoundPush) < time.Second {
		p.roundSyncMu.Unlock()
		return
	}
	p.lastRoundPush = time.Now()
	p.roundSyncMu.Unlock()

	end := stale + 1
	if end > current {
		end = current
	}
	var pushed int
	for r := stale; r <= end; r++ {
		for _, cert := range p.dag.GetRound(r) {
			cert := cert
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				if err := p.gossip.BroadcastCertificate(p.ctx, cert); err != nil {
					slog.Debug("Failed to push certificate", "error", err)
				}
			}()
			pushed++
		}
	}
	if pushed > 0 {
		slog.Info("Pushed certificates for stale round",
			"staleRound", stale, "certificates", pushed)
	}
}
