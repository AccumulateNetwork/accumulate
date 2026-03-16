// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// PendingCertificates buffers certificates that cannot be inserted into the DAG
// because they are missing parent certificates. When a parent becomes available,
// the dependent certificates can be retrieved and retried.
type PendingCertificates struct {
	mu sync.Mutex

	// pending maps certificate digest to the pending entry
	pending map[types.CertificateDigest]*pendingEntry

	// waitingFor maps parent digest to the set of certificates waiting for it
	waitingFor map[types.CertificateDigest]map[types.CertificateDigest]struct{}

	// maxSize is the maximum number of pending certificates to buffer
	maxSize int

	// Metrics
	totalBuffered atomic.Uint64
	totalInserted atomic.Uint64
	totalPruned   atomic.Uint64
}

// pendingEntry represents a certificate waiting for its parents.
type pendingEntry struct {
	cert           *types.Certificate
	missingParents map[types.CertificateDigest]struct{}
	addedAt        time.Time
}

// NewPendingCertificates creates a new pending certificates buffer.
// maxSize limits the number of certificates that can be buffered.
// For a committee with n validators and gc_depth d, a reasonable size is 2*n*d.
func NewPendingCertificates(maxSize int) *PendingCertificates {
	return &PendingCertificates{
		pending:    make(map[types.CertificateDigest]*pendingEntry),
		waitingFor: make(map[types.CertificateDigest]map[types.CertificateDigest]struct{}),
		maxSize:    maxSize,
	}
}

// Add buffers a certificate that is waiting for one or more parent certificates.
// Returns true if the certificate was added, false if the buffer is full or already exists.
func (p *PendingCertificates) Add(cert *types.Certificate, missingParents []types.CertificateDigest) bool {
	if cert == nil || len(missingParents) == 0 {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	digest := cert.Digest()

	// Check if already pending
	if _, exists := p.pending[digest]; exists {
		return false
	}

	// Check buffer size limit
	if len(p.pending) >= p.maxSize {
		return false
	}

	// Create missing parents set
	missing := make(map[types.CertificateDigest]struct{}, len(missingParents))
	for _, parent := range missingParents {
		missing[parent] = struct{}{}

		// Register this certificate as waiting for the parent
		if p.waitingFor[parent] == nil {
			p.waitingFor[parent] = make(map[types.CertificateDigest]struct{})
		}
		p.waitingFor[parent][digest] = struct{}{}
	}

	// Add to pending
	p.pending[digest] = &pendingEntry{
		cert:           cert,
		missingParents: missing,
		addedAt:        time.Now(),
	}

	p.totalBuffered.Add(1)
	return true
}

// OnParentAvailable is called when a parent certificate becomes available.
// It returns any certificates that can now be inserted (all parents available).
func (p *PendingCertificates) OnParentAvailable(parentDigest types.CertificateDigest) []*types.Certificate {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Get certificates waiting for this parent
	waiters, ok := p.waitingFor[parentDigest]
	if !ok || len(waiters) == 0 {
		return nil
	}

	var ready []*types.Certificate

	for waiterDigest := range waiters {
		entry, exists := p.pending[waiterDigest]
		if !exists {
			continue
		}

		// Remove this parent from the missing set
		delete(entry.missingParents, parentDigest)

		// If no more missing parents, this certificate is ready
		if len(entry.missingParents) == 0 {
			ready = append(ready, entry.cert)
			delete(p.pending, waiterDigest)
			p.totalInserted.Add(1)
		}
	}

	// Clean up the waitingFor entry
	delete(p.waitingFor, parentDigest)

	return ready
}

// Prune removes pending certificates that are too old (round < currentRound - gcDepth).
// Returns the number of certificates pruned.
func (p *PendingCertificates) Prune(currentRound types.Round, gcDepth types.Round) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	if currentRound <= gcDepth {
		return 0
	}

	cutoff := currentRound - gcDepth
	pruned := 0

	for digest, entry := range p.pending {
		if entry.cert.Round() < cutoff {
			// Remove from waitingFor
			for parent := range entry.missingParents {
				if waiters, ok := p.waitingFor[parent]; ok {
					delete(waiters, digest)
					if len(waiters) == 0 {
						delete(p.waitingFor, parent)
					}
				}
			}

			delete(p.pending, digest)
			pruned++
		}
	}

	if pruned > 0 {
		p.totalPruned.Add(uint64(pruned))
	}

	return pruned
}

// Size returns the number of pending certificates.
func (p *PendingCertificates) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pending)
}

// Contains returns true if a certificate with the given digest is pending.
func (p *PendingCertificates) Contains(digest types.CertificateDigest) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, exists := p.pending[digest]
	return exists
}

// Metrics returns the total counts of buffered, inserted, and pruned certificates.
func (p *PendingCertificates) Metrics() (buffered, inserted, pruned uint64) {
	return p.totalBuffered.Load(), p.totalInserted.Load(), p.totalPruned.Load()
}

// Get returns a pending certificate by digest, or nil if not found.
func (p *PendingCertificates) Get(digest types.CertificateDigest) *types.Certificate {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, exists := p.pending[digest]; exists {
		return entry.cert
	}
	return nil
}

// MissingParents returns the missing parent digests for a pending certificate.
// Returns nil if the certificate is not pending.
func (p *PendingCertificates) MissingParents(digest types.CertificateDigest) []types.CertificateDigest {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, exists := p.pending[digest]
	if !exists {
		return nil
	}

	parents := make([]types.CertificateDigest, 0, len(entry.missingParents))
	for parent := range entry.missingParents {
		parents = append(parents, parent)
	}
	return parents
}
