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

// OnVoteReceived handles an incoming vote for our header, verifying its
// signature here.
func (p *Primary) OnVoteReceived(vote *types.Vote) {
	p.handleVote(vote, false)
}

// handleVote is OnVoteReceived for a vote the router may have verified:
// one from the votes topic passed its validator, which checked the author's
// membership and signature (#4596), and is not verified again.
func (p *Primary) handleVote(vote *types.Vote, routerVerified bool) {
	if vote == nil {
		return
	}

	p.votesReceived.Add(1)

	// CHEAP CHECK FIRST: Committee membership (~21ns), in the committee of
	// the vote's round.
	// This prevents CPU exhaustion from non-validator vote spam
	committee := p.committeeAt(vote.Round)
	inCommittee := committee.ContainsValidator(vote.Author)
	quorumCount := committee.QuorumCount()

	if !inCommittee {
		slog.Debug("Vote from unknown validator",
			"author", hexEncode(vote.Author))
		return
	}

	// EXPENSIVE CHECK SECOND: Signature verification (~29µs)
	// Only verify signatures from committee members, and only once
	if !routerVerified {
		if err := vote.Verify(); err != nil {
			slog.Debug("Invalid vote signature",
				"error", err,
				"author", hexEncode(vote.Author))
			return
		}
	}
	p.noteHeaderSeen(vote.Author, vote.Round)

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

	if debugEnabled() {
		slog.Debug("Added vote",
			"headerDigest", vote.HeaderDigest.String(),
			"author", hexEncode(vote.Author),
			"totalVotes", len(p.pendingVotes[vote.HeaderDigest]))
	}

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

	// Calculate total stake from votes, in the committee of the header's round
	committee := p.committeeAt(header.Round)
	var totalStake uint64
	for _, v := range votes {
		totalStake += committee.StakeOf(v.Author)
	}
	hasQuorum := committee.HasQuorum(totalStake)
	quorumThreshold := committee.QuorumThreshold()

	// Need 2f+1 stake
	if !hasQuorum {
		slog.Debug("Not enough stake for certificate",
			"headerDigest", headerDigest.String(),
			"totalStake", totalStake,
			"threshold", quorumThreshold)
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

	if debugEnabled() {
		slog.Debug("Created certificate",
			"partition", p.config.Partition,
			"digest", cert.Digest().String(),
			"round", cert.Round(),
			"signers", len(cert.SignedAuthorities))
	}

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
			// Our OWN certificate failed to insert. Without cleanup this
			// node believed it certified the round (ourCerts set, header and
			// votes already deleted) while the network never saw the
			// certificate — and the header's batches were stranded with no
			// requeue (#4159 review A13a). Un-claim the round and put the
			// batches back so they ride the next header.
			slog.Error("Failed to insert OWN certificate into DAG — un-claiming the round and requeuing its batches",
				"error", err,
				"round", cert.Round(),
				"digest", cert.Digest().String())
			p.pendingMu.Lock()
			if c, ok := p.ourCerts[cert.Round()]; ok && c == cert {
				delete(p.ourCerts, cert.Round())
			}
			p.pendingMu.Unlock()
			if cert.Header != nil {
				p.requeueHeaderBatches(cert.Header)
			}
			return
		}

		// In the DAG, and only now: what this node took has reached a
		// certificate (#4366). A node in no committee never arrives here.
		p.countCertifiedOwn(cert)

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

		// Release peers' certificates held waiting for this one: they
		// arrive at the next round, often before it is inserted (#4515).
		p.processPendingForParent(cert.Digest())
	}()
}

// createCertificateFromVotes creates a certificate from the header and
// collected votes, indexing the signers in the committee of the header's
// round.
func (p *Primary) createCertificateFromVotes(header *types.Header, votes []*types.Vote) *types.Certificate {
	if len(votes) == 0 {
		return nil
	}

	sigs := make([][]byte, len(votes))
	authors := make([]uint16, len(votes))

	committee := p.committeeAt(header.Round)
	for i, v := range votes {
		sigs[i] = make([]byte, len(v.Signature))
		copy(sigs[i], v.Signature)

		idx, found := committee.FindValidator(v.Author)
		if !found {
			// This shouldn't happen as we check in OnVoteReceived
			continue
		}
		authors[i] = idx
	}

	return types.NewCertificate(header, sigs, authors)
}

// OnHeaderReceived handles an incoming header from another validator,
// verifying its signature here. If valid, we vote on it.
func (p *Primary) OnHeaderReceived(header *types.Header) {
	p.handleHeader(header, false)
}

// handleHeader is OnHeaderReceived for a header the router may have
// verified: one from the headers topic passed its validator, which checked
// the author's membership and signature (#4596), and is not verified again.
func (p *Primary) handleHeader(header *types.Header, routerVerified bool) {
	if header == nil {
		return
	}

	if debugEnabled() {
		slog.Debug("Header handled by primary",
			"partition", p.config.Partition,
			"author", hexEncode(header.Author),
			"round", header.Round)
	}

	// Check author is in the committee of the header's round: the lookup
	// before the signature check, as for votes
	committee := p.committeeAt(header.Round)
	if !committee.ContainsValidator(header.Author) {
		slog.Debug("Header from unknown validator",
			"author", hexEncode(header.Author))
		return
	}

	// Verify header signature, once
	if !routerVerified {
		if err := header.Verify(); err != nil {
			slog.Debug("Invalid header signature",
				"error", err,
				"author", hexEncode(header.Author))
			return
		}
	}
	p.noteHeaderSeen(header.Author, header.Round)

	// Don't vote on our own headers
	pubKey := p.config.KeyPair.Public().(ed25519.PublicKey)
	if bytes.Equal(header.Author, pubKey) {
		return
	}

	p.roundMu.Lock()
	currentRound := p.currentRound
	p.roundMu.Unlock()

	// The epoch is the one of the header's round
	if header.Epoch != committee.Epoch {
		slog.Debug("Header epoch mismatch",
			"headerEpoch", header.Epoch,
			"roundEpoch", committee.Epoch,
			"round", header.Round)
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

		// A round mismatch after an outage is a deadlock without recovery:
		// certificates are broadcast exactly once, so a node that missed
		// them can neither advance (it rejects newer headers) nor help a
		// stale author advance (the author never learns its round already
		// completed). Sync rounds in both directions (#4057).
		switch {
		case header.Round > maxRound:
			// We are behind. Certificate catch-up can only bridge a gap that
			// peers still retain — up to the DAG's GC depth. A wider gap
			// (a fast-sync rejoin whose seed aged past the horizon before the
			// node restarted, or an outage longer than the horizon) is
			// unbridgeable: the intervening certificates have been collected,
			// so pulling them returns nothing and the node stays pinned at its
			// seed round forever, voting on nothing while gossip floods its
			// channels. Surface that explicitly rather than spinning silently;
			// recovery requires a fresh fast-sync (#4058 fallback).
			if header.Round-currentRound > p.dag.GCDepth() {
				p.warnStrandedBeyondHorizon(currentRound, header.Round)
			} else {
				p.requestRoundCatchUp(currentRound, header.Round)
			}
		case header.Round < minRound:
			// The author is behind — push the certificates it is missing
			p.pushCertsForStaleRound(header.Round, currentRound)
		}
		return
	}

	// No vote above the round limit: the round may belong to a committee
	// change this node has not executed yet. The author rebroadcasts, and
	// the vote goes out once the limit has passed the round.
	if p.beyondLimit(header.Round) {
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

	// One header per author and round (#4590): the first this validator
	// voted for. Refused before anything is asked for on its behalf.
	if p.votedOther(header, headerDigest) {
		return
	}

	// Check we have all parent certificates. Deferring alone is not enough,
	// for exactly the reason the batch gate below spells out: the author's
	// header rebroadcast re-tests a condition that only a re-sent certificate
	// can make true, and a certificate missed once is never re-sent. So ask
	// for it. Without this, one dropped certificate freezes a whole partition
	// -- every validator holds the round's headers, defers every vote on a
	// parent nobody will send again, and the round can never reach the quorum
	// it needs to advance. That is #4182: all four validators of one partition
	// stopped at one round with votes still arriving and nothing going out,
	// while the other partitions ran on. RequestMissing dedupes against the
	// DAG, the in-flight set and the retry bound, so calling it on every
	// rebroadcast costs nothing.
	var missingParents []types.CertificateDigest
	for _, parentDigest := range header.Parents {
		if p.dag.GetByDigest(parentDigest) == nil {
			missingParents = append(missingParents, parentDigest)
		}
	}
	if len(missingParents) > 0 {
		if debugEnabled() {
			slog.Debug("Missing parents for header — deferring vote and fetching",
				"partition", p.config.Partition,
				"headerDigest", headerDigest.String(),
				"round", header.Round,
				"missing", len(missingParents))
		}
		if p.requestParents != nil {
			p.requestParents(missingParents)
		}
		p.deferVote(header, missingParents, nil)
		return // missing parents — asked for them; vote when they land (#4611)
	}

	// The parents are here. The distinct ones of the round before must
	// carry a stake quorum (Narwhal 4.1, #4591); presence alone let a header
	// with no parents through, and a certificate without a path to an
	// earlier leader splits the commit sequence between nodes.
	if err := p.dag.CheckParents(header.Round, header.Parents); err != nil {
		slog.Info("Header's parents do not certify it; not voting",
			"partition", p.config.Partition,
			"author", hexEncode(header.Author),
			"round", header.Round,
			"error", err)
		return
	}

	// Data availability: only vote once we hold every batch this header names
	// (#4159). A certificate must be a proof that 2f+1 validators HAVE the
	// data — that is the premise the whole commit path relies on ("the
	// certificate is proof the data exists"). Voting without the batches
	// breaks it: a header can be certified when only the author holds a batch,
	// and if that batch never propagates to a quorum, Bullshark can commit the
	// leader rounds later and CollectBatches finds the batch nowhere
	// (absence=no-record, peerHits=0 on every node) — a permanent wedge, with
	// zero LRU evictions, exactly as observed at DN 553 / 4152. Missing
	// batches arrive via gossip and the author rebroadcasts the header until
	// it reaches quorum, so we vote on a later rebroadcast once we have them —
	// the same defer-until-available shape as the missing-parent gate above.
	//
	// One call checks and marks: the batches held are marked as named by a
	// header this node votes on, so batches no header names are evicted
	// before them until the certificate protects them (#4598). Checked first
	// and marked after, eviction could take a batch between the two under a
	// flood, and the vote went out for a batch this node no longer held.
	missingBatches := p.nameHeaderBatches(header)
	if len(missingBatches) > 0 {
		// Pin every batch this header names -- including the ones we
		// already hold. Those are the ones eviction takes while we wait
		// for this one, which is what turned a deferred vote into a
		// livelock (#4165).
		p.pinHeaderBatches(headerDigest, header)

		slog.Debug("Missing batches for header — deferring vote and fetching",
			"partition", p.config.Partition,
			"headerDigest", headerDigest.String(),
			"missing", len(missingBatches),
			"of", len(header.Payload),
			"round", header.Round)
		// Deferring alone is not enough: batch bytes are broadcast once
		// at creation and (before #4159) NOTHING re-sent them — the
		// author's 1s header rebroadcast re-tests a condition that
		// could never become true, and the header could never reach
		// quorum. Actively pull the batches (the author certainly holds
		// them — own batches are un-evictable); the next rebroadcast
		// then finds them present and the vote goes out. Every missing
		// batch, not the first: a header authored after a restart
		// re-proposes every batch its node accumulated while it was down,
		// hundreds of them, and one fetch per rebroadcast made certifying
		// it a matter of minutes (run 20261002T090546Z's shape in
		// TestRestartAfterTwoNodesStopFirst).
		for _, d := range missingBatches {
			p.requestMissingBatch(d)
		}
		p.deferVote(header, nil, missingBatches)
		return // missing batches — do not certify data we do not have; vote when they land (#4611)
	}

	// Every batch is here, so nothing is waiting on them any more.
	p.releasePins(headerDigest)
	p.deferred.drop(headerDigest)

	// Record the header as this author's for the round, on disk before the
	// vote exists: a restart must refuse any other header for the pair
	// (#4590). The author rebroadcasts the header until it is certified, so
	// a record that could not be written is tried again then.
	if !p.recordVote(header, headerDigest) {
		return
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

	if debugEnabled() {
		slog.Debug("Voting on header",
			"partition", p.config.Partition,
			"headerDigest", headerDigest.String(),
			"author", hexEncode(header.Author),
			"round", header.Round)
	}

	// Broadcast vote
	p.broadcastVoteAsync(vote, headerDigest)
}

// votedOther reports whether this primary voted for a header other than
// digest from the header's author for its round, or can no longer tell
// because the round's records were pruned (#4590).
func (p *Primary) votedOther(header *types.Header, digest types.HeaderDigest) bool {
	p.pendingMu.Lock()
	voted, ok := p.votedRounds[authorRound{string(header.Author), header.Round}]
	floor := p.votedFloor
	p.pendingMu.Unlock()
	if header.Round < floor {
		slog.Warn("Header for a round whose vote records were pruned; not voting",
			"partition", p.config.Partition,
			"author", hexEncode(header.Author),
			"round", header.Round,
			"floor", floor,
			"header", digest.String())
		return true
	}
	if !ok || voted == digest {
		return false
	}
	slog.Warn("Second header from an author for a round; not voting",
		"partition", p.config.Partition,
		"author", hexEncode(header.Author),
		"round", header.Round,
		"voted", voted.String(),
		"refused", digest.String())
	return true
}

// recordVote records digest as the header this primary votes for from the
// header's author at its round, in memory and through the persistence hook,
// and reports whether the vote may be signed (#4590).
func (p *Primary) recordVote(header *types.Header, digest types.HeaderDigest) bool {
	if p.votedOther(header, digest) {
		return false
	}
	p.pendingMu.Lock()
	p.votedRounds[authorRound{string(header.Author), header.Round}] = digest
	onVoted := p.onVoted
	p.pendingMu.Unlock()
	if onVoted == nil {
		return true
	}
	if err := onVoted(header.Author, header.Round, digest); err != nil {
		slog.Error("Cannot persist the vote record; not voting",
			"partition", p.config.Partition,
			"author", hexEncode(header.Author),
			"round", header.Round,
			"error", err)
		return false
	}
	return true
}

// haveBatch reports whether any of this node's workers holds the batch. A
// gossiped batch is routed to one local worker by digest (not the author's
// worker id carried in the payload entry), so availability must be checked
// across all workers — the same way CollectBatches looks one up (#4159).
func (p *Primary) haveBatch(d types.BatchDigest) bool {
	for _, w := range p.workers {
		if w.CanServeBatch(d) {
			return true
		}
	}
	return false
}

// SetMissingBatchHandler installs the callback the vote gate uses to pull a
// batch it does not hold (#4159). The Node wires this to a peer fetch; the
// handler must not block.
func (p *Primary) SetMissingBatchHandler(fn func(types.BatchDigest)) {
	p.missingBatchMu.Lock()
	defer p.missingBatchMu.Unlock()
	p.onMissingBatch = fn
}

// requestMissingBatch invokes the missing-batch handler at most once per
// batchFetchRetry per digest — the vote gate re-fires on every 1s header
// rebroadcast, and each fetch already tries every connected peer.
func (p *Primary) requestMissingBatch(d types.BatchDigest) {
	const batchFetchRetry = 5 * time.Second
	p.missingBatchMu.Lock()
	fn := p.onMissingBatch
	if fn == nil {
		p.missingBatchMu.Unlock()
		return
	}
	now := time.Now()
	if last, ok := p.missingBatchAsked[d]; ok && now.Sub(last) < batchFetchRetry {
		p.missingBatchMu.Unlock()
		return
	}
	if p.missingBatchAsked == nil {
		p.missingBatchAsked = map[types.BatchDigest]time.Time{}
	}
	// Bound the in-flight map: entries older than the retry window are
	// re-askable anyway, so drop them opportunistically.
	if len(p.missingBatchAsked) > 4096 {
		for k, t := range p.missingBatchAsked {
			if now.Sub(t) >= batchFetchRetry {
				delete(p.missingBatchAsked, k)
			}
		}
	}
	p.missingBatchAsked[d] = now
	p.missingBatchMu.Unlock()

	fn(d)
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

// warnStrandedBeyondHorizon reports, at most once a minute, that this node has
// fallen further behind than certificate catch-up can bridge — the live round
// exceeds its own by more than the DAG's GC depth, so the certificates it would
// need have already been collected by every peer. Without a fresh fast-sync the
// node cannot advance; the warning turns a silent freeze (a seed round pinned
// forever while gossip floods the channels) into a diagnosable one.
func (p *Primary) warnStrandedBeyondHorizon(current, live types.Round) {
	p.roundSyncMu.Lock()
	if time.Since(p.lastStrandedWarn) < time.Minute {
		p.roundSyncMu.Unlock()
		return
	}
	p.lastStrandedWarn = time.Now()
	p.roundSyncMu.Unlock()

	slog.Warn("Stranded beyond the catch-up horizon — a fresh fast-sync is required",
		"partition", p.config.Partition,
		"round", current,
		"liveRound", live,
		"gap", live-current,
		"gcDepth", p.dag.GCDepth())
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
		slog.Debug("Pushed certificates for stale round",
			"staleRound", stale, "certificates", pushed)
	}
}
