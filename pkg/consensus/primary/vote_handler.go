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
		slog.Debug("Vote from unknown validator",
			"author", hexEncode(vote.Author))
		return
	}

	// EXPENSIVE CHECK SECOND: Signature verification (~29µs)
	// Only verify signatures from committee members
	if err := vote.Verify(); err != nil {
		slog.Debug("Invalid vote signature",
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
		slog.Debug("Vote round mismatch",
			"voteRound", vote.Round,
			"headerRound", header.Round)
		return
	}

	if vote.Epoch != header.Epoch {
		slog.Debug("Vote epoch mismatch",
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
	totalVotes := len(p.pendingVotes[vote.HeaderDigest])

	slog.Info("Added vote - attempting certificate",
		"headerDigest", vote.HeaderDigest.String(),
		"author", hexEncode(vote.Author),
		"totalVotes", totalVotes,
		"quorumCount", quorumCount)

	// Try to create certificate
	p.tryCreateCertificateLocked(vote.HeaderDigest)
}

// tryCreateCertificateLocked attempts to create a certificate from collected votes.
// Must be called with p.pendingMu held.
func (p *Primary) tryCreateCertificateLocked(headerDigest types.HeaderDigest) {
	votes := p.pendingVotes[headerDigest]
	header := p.ourHeaders[headerDigest]

	slog.Debug("tryCreateCertificateLocked",
		"headerDigest", headerDigest.String(),
		"votesForThisHeader", len(votes),
		"headerExists", header != nil)

	if header == nil {
		slog.Warn("tryCreateCertificateLocked: header not found",
			"headerDigest", headerDigest.String())
		return
	}

	// Calculate total stake from votes (needs committeeMu for reading stake)
	p.committeeMu.RLock()
	var totalStake uint64
	for i, v := range votes {
		stake := p.committee.StakeOf(v.Author)
		totalStake += stake
		slog.Debug("Vote stake",
			"voteIndex", i,
			"author", hexEncode(v.Author),
			"stake", stake,
			"runningTotal", totalStake)
	}
	hasQuorum := p.committee.HasQuorum(totalStake)
	quorumThreshold := p.committee.QuorumThreshold()
	p.committeeMu.RUnlock()

	// Need 2f+1 stake
	if !hasQuorum {
		slog.Info("Not enough stake for certificate",
			"headerDigest", headerDigest.String(),
			"totalStake", totalStake,
			"threshold", quorumThreshold,
			"numVotes", len(votes),
			"hasQuorum", hasQuorum)
		return
	}

	slog.Info("Quorum achieved - creating certificate",
		"headerDigest", headerDigest.String(),
		"totalStake", totalStake,
		"threshold", quorumThreshold,
		"numVotes", len(votes))

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

	return types.NewCertificate(*header, sigs, authors)
}

// OnHeaderReceived handles an incoming header from another validator.
// If valid, we vote on it.
func (p *Primary) OnHeaderReceived(header *types.Header) {
	if header == nil {
		return
	}

	// Verify header signature
	if err := header.Verify(); err != nil {
		slog.Debug("Invalid header signature",
			"error", err,
			"author", hexEncode(header.Author))
		return
	}

	// Check author is in committee (uses committeeMu)
	p.committeeMu.RLock()
	inCommittee := p.committee.ContainsValidator(header.Author)
	p.committeeMu.RUnlock()

	if !inCommittee {
		slog.Debug("Header from unknown validator",
			"author", hexEncode(header.Author))
		return
	}

	// Don't vote on our own headers
	pubKey := p.config.KeyPair.Public().(ed25519.PublicKey)
	if bytes.Equal(header.Author, pubKey) {
		slog.Info("TRACE: skipping own header",
			"author", hexEncode(header.Author))
		return
	}

	// Check epoch and round (uses roundMu)
	p.roundMu.Lock()
	currentEpoch := p.currentEpoch
	currentRound := p.currentRound
	p.roundMu.Unlock()

	slog.Info("TRACE: epoch/round check",
		"headerEpoch", header.Epoch,
		"currentEpoch", currentEpoch,
		"headerRound", header.Round,
		"currentRound", currentRound)

	// Check epoch matches
	if header.Epoch != currentEpoch {
		slog.Debug("Header epoch mismatch",
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
		slog.Debug("Header round out of range",
			"headerRound", header.Round,
			"currentRound", currentRound,
			"minRound", minRound,
			"maxRound", maxRound)
		return
	}

	// Check if we already voted on this header (uses pendingMu)
	headerDigest := header.Digest()
	p.pendingMu.Lock()
	if _, voted := p.votedHeaders[headerDigest]; voted {
		p.pendingMu.Unlock()
		return
	}
	p.pendingMu.Unlock()

	// Check we have all parent certificates
	for _, parentDigest := range header.Parents {
		if p.dag.GetByDigest(parentDigest) == nil {
			slog.Debug("Missing parent for header",
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

	// Mark as voted (store round for cleanup)
	p.pendingMu.Lock()
	p.votedHeaders[headerDigest] = header.Round
	p.pendingMu.Unlock()

	p.votesSent.Add(1)

	slog.Info("Successfully voting on header",
		"headerDigest", headerDigest.String(),
		"author", hexEncode(header.Author),
		"round", header.Round,
		"epoch", header.Epoch)

	// Broadcast vote
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
