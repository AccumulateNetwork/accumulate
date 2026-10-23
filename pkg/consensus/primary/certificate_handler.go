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
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// OnCertificateReceived handles an incoming certificate from another
// validator, verifying it here.
func (p *Primary) OnCertificateReceived(cert *types.Certificate) {
	p.receiveCertificate(cert, false)
}

// receiveCertificate is OnCertificateReceived reporting what inserting the
// certificate did, which the catch-up path counts (#4469). routerVerified
// says the certificate came through the certs topic, whose validator
// verified it against the committee of its epoch before delivering it
// (#4596, gossip/topic-validators.go): it is not verified again. What
// arrives by any other path, a sync response's certificates among them, is
// verified here.
func (p *Primary) receiveCertificate(cert *types.Certificate, routerVerified bool) CertOutcome {
	if cert == nil {
		return CertFailed
	}

	// Cheap dedup FIRST: during round sync every broadcast response reaches
	// every node, so most certificates are duplicates — checking the DAG
	// costs a map lookup while Verify costs an ed25519 check per signature.
	// Observed without this: 281k duplicate verifications in 3 minutes of
	// catch-up (#4057).
	if held := p.dag.GetByDigest(cert.Digest()); held != nil {
		// Release anything still held waiting for it (#4515): a certificate
		// that reached the DAG by a path that released nothing is released
		// by its next copy.
		p.processPendingForParent(cert.Digest())
		// The digest covers only the header, not even its signature, so a
		// copy is a duplicate only if it is the held certificate as encoded
		// on the wire; a genuine header with other signatures is not
		// verified and does not pass as the certificate it names (#4597).
		if !held.SameWire(cert) {
			slog.Debug("A copy of a held certificate with other signatures",
				"round", cert.Round(),
				"author", hexEncode(cert.Author()),
				"digest", cert.Digest().String())
			return CertInvalid
		}
		return CertDuplicate
	}

	// Refuse before verifying what the buffer would refuse after. A
	// certificate held pending or refused by the full buffer never enters the
	// DAG, so the check above never sees its copies, and every copy -- from
	// gossip, and from each of the peers that answer a sync request -- cost
	// a Verify, an ed25519 check per signature. A joining node, its buffer
	// full of frontier certificates, spent its time verifying refused copies
	// while its sync responses queued and were dropped behind them,
	// including the ones carrying the parents it needed (#4469). Nothing is
	// inserted without verification: a certificate refused here is one the
	// insert path would have refused anyway, and one the node needs (at or
	// below the next round) still takes the full path.
	if p.pendingCerts.Contains(cert.Digest()) {
		return CertBuffered
	}
	if p.pendingCerts.Full() && len(p.findMissingParents(cert)) > 0 {
		p.roundMu.Lock()
		ahead := cert.Round() > p.currentRound+1
		p.roundMu.Unlock()
		if ahead {
			return CertDropped
		}
	}

	// Verify the certificate, unless the router did. Its epoch is still
	// this primary's to judge: the router verifies a certificate against the
	// committee of the epoch it names, which may be the previous one, and
	// Verify's first check refuses any epoch but the committee's.
	if routerVerified {
		p.committeeMu.RLock()
		epoch := p.committee.Epoch
		p.committeeMu.RUnlock()
		if cert.Header.Epoch != epoch {
			slog.Info("Invalid certificate",
				"error", "certificate epoch does not match committee epoch",
				"round", cert.Round(),
				"author", hexEncode(cert.Author()),
				"digest", cert.Digest().String())
			return CertInvalid
		}
		return p.insertCertificateAndProcessPending(cert)
	}
	p.verifications.Add(1)
	metrics.CertVerificationsTotal.WithLabelValues(p.config.Partition).Inc()
	if p.certSyncer != nil {
		p.certSyncer.window.verification()
	}
	if err := cert.Verify(p.committee); err != nil {
		// Info, not Debug: silently dropping peers' certificates stalls
		// round advancement with no externally visible cause (#4054).
		slog.Info("Invalid certificate",
			"error", err,
			"round", cert.Round(),
			"author", hexEncode(cert.Author()),
			"digest", cert.Digest().String())
		return CertInvalid
	}

	// Try to insert and process any newly-unblocked certificates
	return p.insertCertificateAndProcessPending(cert)
}

// insertCertificateAndProcessPending attempts to insert a certificate into the DAG.
// If insertion fails due to missing parents, the certificate is buffered.
// If insertion succeeds, any pending certificates waiting for this one are processed.
func (p *Primary) insertCertificateAndProcessPending(cert *types.Certificate) CertOutcome {
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
					if debugEnabled() {
						slog.Debug("Buffering certificate with missing parents",
							"digest", cert.Digest().String(),
							"round", cert.Round(),
							"author", hexEncode(cert.Author()),
							"missingParents", len(missingParents))
					}

					// Request missing certificates via sync protocol
					if p.requestParents != nil {
						p.requestParents(missingParents)
					}
					// A parent inserted by another goroutine between the
					// failed insert and the Add released nothing, since this
					// certificate was not yet held: release it now (#4515).
					p.releaseInserted(missingParents)
					return CertBuffered
				}
				if p.pendingCerts.Contains(cert.Digest()) {
					return CertBuffered // already held
				}
				// The buffer is full and will not hold it. If this is a
				// certificate the node needs to advance, ask for its parents
				// anyway: re-delivery (round catch-up, stall recovery) brings
				// the certificate back, and without the parents it is refused
				// again every time. After a Rejoin at P the certificates at
				// P+2 name, as weak links, those of P-6..P-1, which no round
				// request asks for, while gossip fills the buffer with
				// frontier certificates it never prunes -- a follower's
				// Directory sat at P+2 for four minutes (#4469). Certificates
				// further ahead are left to round catch-up.
				p.roundMu.Lock()
				needed := cert.Round() <= p.currentRound+1
				p.roundMu.Unlock()
				if needed && p.requestParents != nil {
					p.requestParents(missingParents)
				}
				return CertDropped
			}
			return CertFailed
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
			return CertEquivocation
		}
		// Duplicates are routine — during round sync every broadcast response
		// reaches every node, so most certificates have already been inserted
		if strings.Contains(err.Error(), "already exists") {
			slog.Debug("Duplicate certificate", "round", cert.Round())
			return CertDuplicate
		}
		slog.Info("Failed to insert certificate into DAG",
			"error", err,
			"round", cert.Round(),
			"digest", cert.Digest().String())
		return CertFailed
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
	return CertInserted
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

		if debugEnabled() {
			slog.Debug("Inserted previously-pending certificate",
				"digest", cert.Digest().String(),
				"round", cert.Round(),
				"author", hexEncode(cert.Author()))
		}

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
		if !p.lastRoundAdvance.IsZero() {
			metrics.RoundDurationSeconds.Observe(now.Sub(p.lastRoundAdvance).Seconds())
		}
		p.lastRoundAdvance = now
		p.roundMu.Unlock()
		advanced = true

		slog.Debug("Advanced to new round",
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

	// The backstop for a missed release, once a second: the round tick is
	// 50 ms, and the scan allocates every held certificate's parents.
	if time.Since(p.lastReleaseScan) >= time.Second {
		p.lastReleaseScan = time.Now()
		p.releaseInserted(p.pendingCerts.GetAllMissingParents())
	}
	pruned := p.pendingCerts.Prune(currentRound, DefaultPendingCertsGCDepth)
	if pruned > 0 {
		slog.Debug("Pruned old pending certificates",
			"count", pruned,
			"currentRound", currentRound)
	}
}

// ForgetHeld drops every certificate held for want of a parent, and reports
// how many: what a node seeded by Node.Rejoin held before the seed
// (PendingCertificates.Forget).
func (p *Primary) ForgetHeld() int {
	return p.pendingCerts.Forget()
}

// PendingCertsCount returns the number of certificates waiting for missing parents.
func (p *Primary) PendingCertsCount() int {
	return p.pendingCerts.Size()
}

// PendingCertsMetrics returns metrics about the pending certificates buffer.
func (p *Primary) PendingCertsMetrics() (buffered, inserted, pruned uint64) {
	return p.pendingCerts.Metrics()
}

// PendingDiagnosis reports, for diagnosis, how the pending buffer stands:
// how many certificates it holds, how many of those wait only on parents
// already in the DAG (a wake-up that never came), and how many parents it
// waits on that are neither in the DAG nor held (what the node lacks).
func (p *Primary) PendingDiagnosis() (held, waitingOnInserted, lacking int) {
	lack := map[types.CertificateDigest]struct{}{}
	p.pendingCerts.forEach(func(_ types.CertificateDigest, missing map[types.CertificateDigest]struct{}) {
		held++
		all := len(missing) > 0
		for d := range missing {
			if !p.dag.Contains(d) {
				all = false
				if !p.pendingCerts.containsLocked(d) {
					lack[d] = struct{}{}
				}
			}
		}
		if all {
			waitingOnInserted++
		}
	})
	return held, waitingOnInserted, len(lack)
}

// releaseInserted releases the certificates held waiting for any of parents
// that is in the DAG. A held certificate waits until a release reaches it,
// and a copy of it that arrives again is reported held without retrying, so
// a missed release is permanent. Releases were missed (#4515): this node's
// own certificates are inserted on a goroutine that released nothing, a
// duplicate released nothing, and a parent inserted by another goroutine
// between a child's failed insert and its Add released nothing. consim
// TestRestartOneDirectoryValidatorOfTwelve wedged with 240 held: one waited
// only on parents already in the DAG, and the rest waited on it. This runs
// after each Add for that certificate's parents, and on every round tick
// over everything held, as the backstop that makes any missed release
// temporary.
func (p *Primary) releaseInserted(parents []types.CertificateDigest) {
	for _, d := range parents {
		if p.dag.Contains(d) {
			p.processPendingForParent(d)
		}
	}
}
