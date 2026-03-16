// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"crypto/ed25519"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// createHeaderLocked creates a header for the current round.
// Must be called with p.mu held.
func (p *Primary) createHeaderLocked() (*types.Header, error) {
	// 1. FIRST: Check parent certificates exist
	// We check parents before consuming batches to avoid losing batches
	// if the parent check fails (e.g., not enough parents in round-1).
	parents, err := p.getParentCertsLocked()
	if err != nil {
		return nil, err
	}

	// 2. THEN: Collect available batch digests from workers
	// Now that we know parents exist, it's safe to consume batches.
	payload := make(map[types.BatchDigest]types.WorkerID)
	for _, w := range p.workers {
		// Use ConsumeAvailableBatches to get and clear available batches
		for _, digest := range w.ConsumeAvailableBatches() {
			payload[digest] = w.ID()
		}
	}

	// 3. Build header
	pubKey := p.config.KeyPair.Public().(ed25519.PublicKey)
	header := types.NewHeader(pubKey, p.currentRound, p.currentEpoch, payload, parents)

	// 4. Sign
	if err := header.Sign(p.config.KeyPair); err != nil {
		return nil, fmt.Errorf("sign header: %w", err)
	}

	return header, nil
}

// getParentCertsLocked retrieves 2f+1 parent certificates from round-1.
// Must be called with p.mu held.
func (p *Primary) getParentCertsLocked() ([]types.CertificateDigest, error) {
	if p.currentRound == 0 {
		return nil, nil // genesis round has no parents
	}

	certs := p.dag.GetRound(p.currentRound - 1)
	quorumCount := p.committee.QuorumCount()

	if len(certs) < quorumCount {
		return nil, fmt.Errorf("%w: have %d, need %d",
			ErrNotEnoughParents, len(certs), quorumCount)
	}

	// Collect digests from certificates, up to quorum
	// We could take all, but quorum is sufficient
	digests := make([]types.CertificateDigest, 0, len(certs))
	for _, cert := range certs {
		digests = append(digests, cert.Digest())
	}

	return digests, nil
}

// CreateHeader creates a header for the current round (thread-safe).
// Returns an error if not enough parent certificates are available.
func (p *Primary) CreateHeader() (*types.Header, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.createHeaderLocked()
}

// GetParentCerts returns the parent certificate digests for the current round.
func (p *Primary) GetParentCerts() ([]types.CertificateDigest, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.getParentCertsLocked()
}

// AvailableBatches returns all available batch digests from workers.
func (p *Primary) AvailableBatches() map[types.BatchDigest]types.WorkerID {
	payload := make(map[types.BatchDigest]types.WorkerID)
	for _, w := range p.workers {
		// Use AvailableBatches to peek without consuming
		for _, digest := range w.AvailableBatches() {
			payload[digest] = w.ID()
		}
	}
	return payload
}
