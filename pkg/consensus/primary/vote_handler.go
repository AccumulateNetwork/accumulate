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

	// Verify vote signature
	if err := vote.Verify(); err != nil {
		slog.Debug("Invalid vote signature",
			"error", err,
			"author", hexEncode(vote.Author))
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

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

	// Check voter is in committee
	if !p.committee.ContainsValidator(vote.Author) {
		slog.Debug("Vote from unknown validator",
			"author", hexEncode(vote.Author))
		return
	}

	// Add vote (avoid duplicates)
	votes := p.pendingVotes[vote.HeaderDigest]
	for _, v := range votes {
		if bytes.Equal(v.Author, vote.Author) {
			return // already have vote from this author
		}
	}
	p.pendingVotes[vote.HeaderDigest] = append(votes, vote)

	slog.Debug("Added vote",
		"headerDigest", vote.HeaderDigest.String(),
		"author", hexEncode(vote.Author),
		"totalVotes", len(p.pendingVotes[vote.HeaderDigest]))

	// Try to create certificate
	p.tryCreateCertificateLocked(vote.HeaderDigest)
}

// tryCreateCertificateLocked attempts to create a certificate from collected votes.
// Must be called with p.mu held.
func (p *Primary) tryCreateCertificateLocked(headerDigest types.HeaderDigest) {
	votes := p.pendingVotes[headerDigest]
	header := p.ourHeaders[headerDigest]

	if header == nil {
		return
	}

	// Calculate total stake from votes
	var totalStake uint64
	for _, v := range votes {
		totalStake += p.committee.StakeOf(v.Author)
	}

	// Need 2f+1 stake
	if !p.committee.HasQuorum(totalStake) {
		slog.Debug("Not enough stake for certificate",
			"headerDigest", headerDigest.String(),
			"totalStake", totalStake,
			"threshold", p.committee.QuorumThreshold())
		return
	}

	// Create certificate
	cert := p.createCertificateFromVotes(header, votes)
	if cert == nil {
		return
	}

	// Store in our certs
	p.ourCerts[cert.Round()] = cert
	p.certificatesCreated.Add(1)

	slog.Info("Created certificate",
		"digest", cert.Digest().String(),
		"round", cert.Round(),
		"signers", len(cert.SignedAuthorities))

	// Clean up pending state for this header
	delete(p.pendingVotes, headerDigest)
	delete(p.ourHeaders, headerDigest)

	// Insert into DAG and broadcast (outside lock)
	go func() {
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
func (p *Primary) createCertificateFromVotes(header *types.Header, votes []*types.Vote) *types.Certificate {
	if len(votes) == 0 {
		return nil
	}

	sigs := make([][]byte, len(votes))
	authors := make([]uint16, len(votes))

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

	// Clone the header to avoid sharing state
	headerClone := header.Clone()

	return types.NewCertificate(*headerClone, sigs, authors)
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

	// Check author is in committee
	if !p.committee.ContainsValidator(header.Author) {
		slog.Debug("Header from unknown validator",
			"author", hexEncode(header.Author))
		return
	}

	// Don't vote on our own headers
	pubKey := p.config.KeyPair.Public().(ed25519.PublicKey)
	if bytes.Equal(header.Author, pubKey) {
		return
	}

	p.mu.Lock()

	// Check epoch matches
	if header.Epoch != p.currentEpoch {
		p.mu.Unlock()
		slog.Debug("Header epoch mismatch",
			"headerEpoch", header.Epoch,
			"currentEpoch", p.currentEpoch)
		return
	}

	// Check round is acceptable (not too old, not too far ahead)
	minRound := p.currentRound
	if minRound > 1 {
		minRound = p.currentRound - 1
	} else {
		minRound = 0
	}
	maxRound := p.currentRound + 1

	if header.Round < minRound || header.Round > maxRound {
		p.mu.Unlock()
		slog.Debug("Header round out of range",
			"headerRound", header.Round,
			"currentRound", p.currentRound,
			"minRound", minRound,
			"maxRound", maxRound)
		return
	}

	// Check if we already voted on this header
	headerDigest := header.Digest()
	if _, voted := p.votedHeaders[headerDigest]; voted {
		p.mu.Unlock()
		return
	}

	p.mu.Unlock()

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
	p.mu.Lock()
	p.votedHeaders[headerDigest] = header.Round
	p.mu.Unlock()

	p.votesSent.Add(1)

	slog.Debug("Voting on header",
		"headerDigest", headerDigest.String(),
		"author", hexEncode(header.Author),
		"round", header.Round)

	// Broadcast vote
	go func() {
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
