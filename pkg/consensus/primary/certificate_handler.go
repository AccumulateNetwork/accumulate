// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// OnCertificateReceived handles an incoming certificate from another validator.
func (p *Primary) OnCertificateReceived(cert *types.Certificate) {
	if cert == nil {
		return
	}

	// Cheap dedup FIRST: during round sync every broadcast response reaches
	// every node, so most certificates are duplicates — checking the DAG
	// costs a map lookup while Verify costs an ed25519 check per signature.
	// Observed without this: 281k duplicate verifications in 3 minutes of
	// catch-up (#4057).
	if p.dag.Contains(cert.Digest()) {
		return
	}

	// Verify certificate
	if err := cert.Verify(p.committee); err != nil {
		// Info, not Debug: silently dropping peers' certificates stalls
		// round advancement with no externally visible cause (#4054).
		slog.Info("Invalid certificate",
			"error", err,
			"round", cert.Round(),
			"author", hexEncode(cert.Author()),
			"digest", cert.Digest().String())
		return
	}

	// Try to insert and process any newly-unblocked certificates
	p.insertCertificateAndProcessPending(cert)
}

// insertCertificateAndProcessPending attempts to insert a certificate into the DAG.
// If insertion fails due to missing parents, the certificate is buffered.
// If insertion succeeds, any pending certificates waiting for this one are processed.
func (p *Primary) insertCertificateAndProcessPending(cert *types.Certificate) {
	// Insert into DAG (validates parents exist)
	var err error
	if cert.Round() == 0 {
		err = p.dag.InsertGenesis(cert)
	} else {
		err = p.dag.Insert(cert)
	}

	if err != nil {
		// Check if this is a "parent not found" error
		if strings.Contains(err.Error(), "parent certificate not found in DAG") {
			// Find which parents are missing
			missingParents := p.findMissingParents(cert)
			if len(missingParents) > 0 {
				// Buffer the certificate
				if p.pendingCerts.Add(cert, missingParents) {
					slog.Info("Buffering certificate with missing parents",
						"digest", cert.Digest().String(),
						"round", cert.Round(),
						"author", hexEncode(cert.Author()),
						"missingParents", len(missingParents))

					// Request missing certificates via sync protocol
					if p.certSyncer != nil {
						p.certSyncer.RequestMissing(missingParents)
					}
				}
			}
			return
		}

		// EQUIVOCATION FIRST: a different certificate for the same author
		// and round is a dual-signer or a serious bug, and it must be LOUD.
		// This used to be classified by matching "already exists" in the
		// message — a substring the equivocation error itself contains — so
		// the tripwire was silently downgraded to the duplicate-Debug below
		// and never fired.
		if errors.Is(err, dag.ErrEquivocation) {
			slog.Error("EQUIVOCATION detected — conflicting certificate rejected",
				"error", err,
				"round", cert.Round(),
				"author", hexEncode(cert.Author()),
				"digest", cert.Digest().String())
			return
		}
		// Duplicates are routine — during round sync every broadcast response
		// reaches every node, so most certificates have already been inserted
		if strings.Contains(err.Error(), "already exists") {
			slog.Debug("Duplicate certificate", "round", cert.Round())
			return
		}
		slog.Info("Failed to insert certificate into DAG",
			"error", err,
			"round", cert.Round(),
			"digest", cert.Digest().String())
		return
	}

	slog.Debug("Inserted certificate from peer",
		"digest", cert.Digest().String(),
		"round", cert.Round(),
		"author", hexEncode(cert.Author()))

	// Clear from in-flight sync tracking (cert may have arrived via normal gossip)
	if p.certSyncer != nil {
		p.certSyncer.ClearInFlight(cert.Digest())
	}

	// Signal for Bullshark
	p.signalNewCertificate(cert)

	// Check if any pending certificates can now be inserted
	p.processPendingForParent(cert.Digest())

	// Maybe we can advance round now
	p.tryAdvanceRound()
}

// findMissingParents returns the parent digests that are not present in the DAG.
func (p *Primary) findMissingParents(cert *types.Certificate) []types.CertificateDigest {
	var missing []types.CertificateDigest
	for _, parentDigest := range cert.Parents() {
		if !p.dag.Contains(parentDigest) {
			missing = append(missing, parentDigest)
		}
	}
	return missing
}

// processPendingForParent checks if any pending certificates can now be inserted
// after a parent certificate became available. This is recursive - if inserting
// a previously-pending certificate succeeds, it may unblock more certificates.
func (p *Primary) processPendingForParent(parentDigest types.CertificateDigest) {
	// Get certificates that were waiting for this parent
	readyCerts := p.pendingCerts.OnParentAvailable(parentDigest)

	for _, cert := range readyCerts {
		slog.Debug("Retrying previously-pending certificate",
			"digest", cert.Digest().String(),
			"round", cert.Round())

		// Try to insert (may fail if there are other missing parents that
		// weren't tracked, or succeed and trigger more processing)
		var err error
		if cert.Round() == 0 {
			err = p.dag.InsertGenesis(cert)
		} else {
			err = p.dag.Insert(cert)
		}

		if err != nil {
			// Check if still missing parents
			if strings.Contains(err.Error(), "parent certificate not found in DAG") {
				missingParents := p.findMissingParents(cert)
				if len(missingParents) > 0 {
					// Re-buffer with updated missing parents
					p.pendingCerts.Add(cert, missingParents)

					// Re-request the missing parents - critical for recovery
					if p.certSyncer != nil {
						p.certSyncer.RequestMissing(missingParents)
					}
				}
			} else {
				slog.Debug("Failed to insert previously-pending certificate",
					"error", err,
					"digest", cert.Digest().String())
			}
			continue
		}

		slog.Info("Inserted previously-pending certificate",
			"digest", cert.Digest().String(),
			"round", cert.Round(),
			"author", hexEncode(cert.Author()))

		// Clear from in-flight sync tracking
		if p.certSyncer != nil {
			p.certSyncer.ClearInFlight(cert.Digest())
		}

		// Signal for Bullshark
		p.signalNewCertificate(cert)

		// Recursively process any certificates waiting for this one
		p.processPendingForParent(cert.Digest())
	}
}

// tryAdvanceRound attempts to advance to the next round if we have enough certificates.
func (p *Primary) tryAdvanceRound() {
	advanced := false
	for {
		// Get current round and committee for quorum check
		p.roundMu.Lock()
		currentRound := p.currentRound
		p.roundMu.Unlock()

		p.committeeMu.RLock()
		committee := p.committee
		p.committeeMu.RUnlock()

		// Can advance when we have 2f+1 certificates in current round
		if !p.dag.HasQuorum(currentRound, committee) {
			break
		}

		// Now take roundMu to update round state
		p.roundMu.Lock()
		// Re-check current round in case it changed
		if p.currentRound != currentRound {
			p.roundMu.Unlock()
			return
		}

		// Rate limit: don't advance faster than MinRoundInterval — but only at
		// the frontier. When the DAG already holds certificates PAST our round,
		// the network has moved on without us, and pacing the catch-up is an
		// absorbing state: we advance at exactly the same rate as the frontier,
		// permanently offset, so every header we author is stale on arrival,
		// nobody votes for it, and every transaction our workers batch is lost.
		// Run 20260820T060039Z: 4 of 12 validators pinned ~4 rounds behind for
		// the whole run — BVN2 lost anchor quorum (2-of-4 signers) and its
		// entire outbound stream stalled. Catching up to certificates that
		// already exist cannot accelerate the network; the frontier is paced by
		// the quorum that formed it.
		behind := p.dag.LatestRound() > currentRound
		now := time.Now()
		if !behind && !p.lastRoundAdvance.IsZero() {
			elapsed := now.Sub(p.lastRoundAdvance)
			if elapsed < p.config.MinRoundInterval {
				// Too soon, wait for the ticker to try again
				p.roundMu.Unlock()
				break
			}
		}

		oldRound := p.currentRound
		p.currentRound++
		p.lastRoundAdvance = now
		p.roundMu.Unlock()
		advanced = true

		slog.Info("Advanced to new round",
			"partition", p.config.Partition,
			"oldRound", oldRound,
			"newRound", oldRound+1,
			"catchUp", behind)

		// While behind, keep advancing — full rounds are already in the DAG.
		// Skip authoring headers for the intermediate rounds: they would be
		// stale on arrival, and a validator need not author every round.
		if !behind {
			break
		}
	}
	if !advanced {
		return
	}

	// Clean up old headers
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

		p.cleanupOldHeaders()
	}()

	// Create header for new round (outside lock to avoid deadlock)
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

		p.tryCreateAndBroadcastHeader()
	}()
}

// AdvanceRound forcibly advances to the next round.
// This is useful for testing or when manual round advancement is needed.
func (p *Primary) AdvanceRound() {
	p.roundMu.Lock()
	oldRound := p.currentRound
	p.currentRound++
	newRound := p.currentRound
	p.roundMu.Unlock()

	slog.Info("Forcibly advanced to new round",
		"oldRound", oldRound,
		"newRound", newRound)

	p.tryCreateAndBroadcastHeader()
}

// SetRound sets the current round (useful for testing or sync).
func (p *Primary) SetRound(round types.Round) {
	p.roundMu.Lock()
	p.currentRound = round
	p.roundMu.Unlock()
}

// SetEpoch sets the current epoch (useful for committee changes).
func (p *Primary) SetEpoch(epoch uint64) {
	p.roundMu.Lock()
	p.currentEpoch = epoch
	p.roundMu.Unlock()
}

// UpdateCommittee updates the committee (for epoch transitions).
// This method handles epoch transitions by updating the committee and epoch,
// recalculating dependent values, and ensuring certificate validity across epochs.
func (p *Primary) UpdateCommittee(committee *types.Committee) {
	// Lock both committee and round mutexes (always in same order to avoid deadlock)
	p.committeeMu.Lock()
	p.roundMu.Lock()

	oldEpoch := p.currentEpoch
	oldCommittee := p.committee

	// Update committee and epoch
	p.committee = committee
	p.currentEpoch = committee.Epoch

	p.roundMu.Unlock()
	p.committeeMu.Unlock()

	// Log the transition
	slog.Info("Updated committee",
		"oldEpoch", oldEpoch,
		"newEpoch", committee.Epoch,
		"validators", len(committee.Validators))

	// If this is an epoch transition, perform additional cleanup
	if committee.Epoch > oldEpoch {
		// Compute and log validator changes
		if oldCommittee != nil {
			diff := oldCommittee.DiffWith(committee)
			for _, update := range diff {
				switch update.Type {
				case types.ValidatorUpdateAdd:
					slog.Info("Validator added",
						"pubkey", hexEncode(update.PublicKey),
						"stake", update.NewStake)
				case types.ValidatorUpdateRemove:
					slog.Info("Validator removed",
						"pubkey", hexEncode(update.PublicKey))
				case types.ValidatorUpdateStake:
					slog.Info("Validator stake changed",
						"pubkey", hexEncode(update.PublicKey),
						"newStake", update.NewStake)
				}
			}
		}
	}
}

// TransitionEpoch performs a complete epoch transition with the given validator updates.
// This creates a new committee by applying the updates and sets it as the current committee.
// The round is optionally reset based on the resetRound parameter.
func (p *Primary) TransitionEpoch(updates []types.ValidatorUpdate, resetRound bool) {
	// Lock both committee and round mutexes (always in same order to avoid deadlock)
	p.committeeMu.Lock()
	p.roundMu.Lock()

	oldEpoch := p.currentEpoch
	oldRound := p.currentRound

	// Create new committee by applying updates
	newCommittee := p.committee.UpdateValidators(updates)

	// Validate the new committee
	if err := newCommittee.Validate(); err != nil {
		p.roundMu.Unlock()
		p.committeeMu.Unlock()
		slog.Error("Invalid committee after epoch transition",
			"error", err,
			"epoch", newCommittee.Epoch)
		return
	}

	// Update committee and epoch
	p.committee = newCommittee
	p.currentEpoch = newCommittee.Epoch

	// Optionally reset round for new epoch
	if resetRound {
		p.currentRound = 0
		p.lastRoundAdvance = time.Time{}
	}

	currentRound := p.currentRound
	p.roundMu.Unlock()
	p.committeeMu.Unlock()

	slog.Info("Completed epoch transition",
		"oldEpoch", oldEpoch,
		"newEpoch", newCommittee.Epoch,
		"oldRound", oldRound,
		"currentRound", currentRound,
		"validators", len(newCommittee.Validators),
		"totalStake", newCommittee.TotalStake(),
		"quorumThreshold", newCommittee.QuorumThreshold())

	// Log individual updates
	for _, update := range updates {
		switch update.Type {
		case types.ValidatorUpdateAdd:
			slog.Info("Validator added",
				"pubkey", hexEncode(update.PublicKey),
				"stake", update.NewStake)
		case types.ValidatorUpdateRemove:
			slog.Info("Validator removed",
				"pubkey", hexEncode(update.PublicKey))
		case types.ValidatorUpdateStake:
			slog.Info("Validator stake changed",
				"pubkey", hexEncode(update.PublicKey),
				"newStake", update.NewStake)
		}
	}
}

// Committee returns the current committee (thread-safe).
func (p *Primary) Committee() *types.Committee {
	p.committeeMu.RLock()
	defer p.committeeMu.RUnlock()
	return p.committee
}

// GetOurCertificate returns our certificate for the given round, if any.
func (p *Primary) GetOurCertificate(round types.Round) *types.Certificate {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()

	return p.ourCerts[round]
}

// HasCertificateForRound returns true if we have created a certificate for the round.
func (p *Primary) HasCertificateForRound(round types.Round) bool {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()

	_, ok := p.ourCerts[round]
	return ok
}

// PendingVoteCount returns the number of pending votes for a header.
func (p *Primary) PendingVoteCount(headerDigest types.HeaderDigest) int {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()

	return len(p.pendingVotes[headerDigest])
}

// OurHeadersCount returns the number of headers we're collecting votes for.
func (p *Primary) OurHeadersCount() int {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()

	return len(p.ourHeaders)
}

// prunePendingCerts removes old pending certificates that can no longer be inserted.
func (p *Primary) prunePendingCerts() {
	p.roundMu.Lock()
	currentRound := p.currentRound
	p.roundMu.Unlock()

	pruned := p.pendingCerts.Prune(currentRound, DefaultPendingCertsGCDepth)
	if pruned > 0 {
		slog.Debug("Pruned old pending certificates",
			"count", pruned,
			"currentRound", currentRound)
	}
}

// PendingCertsCount returns the number of certificates waiting for missing parents.
func (p *Primary) PendingCertsCount() int {
	return p.pendingCerts.Size()
}

// PendingCertsMetrics returns metrics about the pending certificates buffer.
func (p *Primary) PendingCertsMetrics() (buffered, inserted, pruned uint64) {
	return p.pendingCerts.Metrics()
}
