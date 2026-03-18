// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dag

import (
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// IsAncestor checks if ancestor is reachable from descendant by following parent links.
// Returns true if there is a path from descendant to ancestor in the DAG.
func (d *DAG) IsAncestor(ancestor, descendant *types.Certificate) bool {
	if ancestor == nil || descendant == nil {
		return false
	}

	// Ancestor must be from an earlier round
	if ancestor.Round() >= descendant.Round() {
		return ancestor.Digest() == descendant.Digest()
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.isAncestorLocked(ancestor, descendant)
}

// isAncestorLocked performs the ancestor check assuming the lock is held.
func (d *DAG) isAncestorLocked(ancestor, descendant *types.Certificate) bool {
	ancestorDigest := ancestor.Digest()
	ancestorRound := ancestor.Round()

	// BFS from descendant to ancestor
	visited := make(map[types.CertificateDigest]bool)
	queue := []*types.Certificate{descendant}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current.Digest()] {
			continue
		}
		visited[current.Digest()] = true

		// Check if we found the ancestor
		if current.Digest() == ancestorDigest {
			return true
		}

		// Don't go past the ancestor's round
		if current.Round() <= ancestorRound {
			continue
		}

		// Add parents to queue
		for _, parentDigest := range current.Parents() {
			parentCert := d.getByDigestLocked(parentDigest)
			if parentCert != nil && !visited[parentDigest] {
				queue = append(queue, parentCert)
			}
		}
	}

	return false
}

// getByDigestLocked retrieves a certificate by digest without locking.
// Uses O(1) lookup via the digest index.
func (d *DAG) getByDigestLocked(digest types.CertificateDigest) *types.Certificate {
	return d.digestIndex[digest]
}

// GetAncestors returns all certificates reachable from cert down to minRound.
// The result includes cert itself and all its transitive parents.
func (d *DAG) GetAncestors(cert *types.Certificate, minRound types.Round) []*types.Certificate {
	if cert == nil {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	visited := make(map[types.CertificateDigest]bool)
	var result []*types.Certificate

	// DFS to collect all ancestors
	stack := []*types.Certificate{cert}

	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		digest := current.Digest()
		if visited[digest] {
			continue
		}
		visited[digest] = true
		result = append(result, current)

		// Don't go past minRound
		if current.Round() <= minRound {
			continue
		}

		// Add parents to stack
		for _, parentDigest := range current.Parents() {
			parentCert := d.getByDigestLocked(parentDigest)
			if parentCert != nil && !visited[parentDigest] {
				stack = append(stack, parentCert)
			}
		}
	}

	return result
}

// TopologicalSort returns certificates in topological order (parents before children).
// Certificates from lower rounds come before certificates from higher rounds.
// Within the same round, certificates are sorted by digest for determinism.
func (d *DAG) TopologicalSort(certs []*types.Certificate) []*types.Certificate {
	if len(certs) == 0 {
		return nil
	}

	// Build a set of certificates to sort
	certSet := make(map[types.CertificateDigest]*types.Certificate)
	for _, cert := range certs {
		certSet[cert.Digest()] = cert
	}

	// Build dependency graph (parents that are in certSet)
	inDegree := make(map[types.CertificateDigest]int)
	children := make(map[types.CertificateDigest][]types.CertificateDigest)

	for digest := range certSet {
		inDegree[digest] = 0
		children[digest] = nil
	}

	for digest, cert := range certSet {
		for _, parentDigest := range cert.Parents() {
			if _, ok := certSet[parentDigest]; ok {
				inDegree[digest]++
				children[parentDigest] = append(children[parentDigest], digest)
			}
		}
	}

	// Kahn's algorithm with round-based ordering
	var result []*types.Certificate
	var ready []types.CertificateDigest

	// Start with certificates that have no dependencies in the set
	for digest, degree := range inDegree {
		if degree == 0 {
			ready = append(ready, digest)
		}
	}

	for len(ready) > 0 {
		// Sort ready list by round (ascending), then by digest for determinism
		sort.Slice(ready, func(i, j int) bool {
			certI := certSet[ready[i]]
			certJ := certSet[ready[j]]
			if certI.Round() != certJ.Round() {
				return certI.Round() < certJ.Round()
			}
			return ready[i].String() < ready[j].String()
		})

		// Take the first (lowest round, then lowest digest)
		digest := ready[0]
		ready = ready[1:]

		cert := certSet[digest]
		result = append(result, cert)

		// Decrease in-degree of children
		for _, childDigest := range children[digest] {
			inDegree[childDigest]--
			if inDegree[childDigest] == 0 {
				ready = append(ready, childDigest)
			}
		}
	}

	// Check for cycles (shouldn't happen in a valid DAG)
	if len(result) != len(certs) {
		// Return partial result; caller should handle this
		return result
	}

	return result
}

// GetChildren returns all certificates that reference cert as a parent.
// This is useful for forward traversal.
func (d *DAG) GetChildren(cert *types.Certificate) []*types.Certificate {
	if cert == nil {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	certDigest := cert.Digest()
	nextRound := cert.Round() + 1

	var children []*types.Certificate

	// Check certificates in the next round
	if roundMap, ok := d.rounds[nextRound]; ok {
		for _, child := range roundMap {
			for _, parentDigest := range child.Parents() {
				if parentDigest == certDigest {
					children = append(children, child)
					break
				}
			}
		}
	}

	return children
}

// GetDescendants returns all certificates reachable from cert going forward.
// This includes cert itself and all certificates that directly or indirectly reference it.
func (d *DAG) GetDescendants(cert *types.Certificate, maxRound types.Round) []*types.Certificate {
	if cert == nil {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	visited := make(map[types.CertificateDigest]bool)
	var result []*types.Certificate

	// BFS forward
	queue := []*types.Certificate{cert}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		digest := current.Digest()
		if visited[digest] {
			continue
		}
		visited[digest] = true
		result = append(result, current)

		// Don't go past maxRound
		if current.Round() >= maxRound {
			continue
		}

		// Find children (certificates in next round that reference this one)
		nextRound := current.Round() + 1
		if roundMap, ok := d.rounds[nextRound]; ok {
			for _, child := range roundMap {
				for _, parentDigest := range child.Parents() {
					if parentDigest == digest && !visited[child.Digest()] {
						queue = append(queue, child)
						break
					}
				}
			}
		}
	}

	return result
}

// FindLeader returns the certificate authored by the leader for a given round.
// The leader is selected deterministically based on the round number.
// This is a simple round-robin selection; actual implementations may use
// stake-weighted selection.
func (d *DAG) FindLeader(round types.Round, committee *types.Committee) *types.Certificate {
	if committee == nil || committee.Len() == 0 {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	// Simple round-robin leader selection
	leaderIdx := int(round) % committee.Len()
	leaderPubKey := committee.Validators[leaderIdx].PublicKey

	roundMap, ok := d.rounds[round]
	if !ok {
		return nil
	}

	for _, cert := range roundMap {
		if types.ValidatorsEqual(cert.Author(), leaderPubKey) {
			return cert
		}
	}

	return nil
}

// CountSupport counts how many certificates in the given round reference
// the specified certificate as an ancestor.
func (d *DAG) CountSupport(cert *types.Certificate, round types.Round, committee *types.Committee) uint64 {
	if cert == nil || round <= cert.Round() {
		return 0
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	roundMap, ok := d.rounds[round]
	if !ok {
		return 0
	}

	var totalStake uint64
	certDigest := cert.Digest()

	for _, supporter := range roundMap {
		// Check if supporter references cert (directly or transitively)
		if d.hasPathLocked(supporter, certDigest, cert.Round()) {
			totalStake += committee.StakeOf(supporter.Author())
		}
	}

	return totalStake
}

// hasPathLocked checks if there's a path from cert to targetDigest.
func (d *DAG) hasPathLocked(cert *types.Certificate, targetDigest types.CertificateDigest, targetRound types.Round) bool {
	if cert.Digest() == targetDigest {
		return true
	}

	if cert.Round() <= targetRound {
		return false
	}

	visited := make(map[types.CertificateDigest]bool)
	stack := []*types.Certificate{cert}

	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		for _, parentDigest := range current.Parents() {
			if parentDigest == targetDigest {
				return true
			}

			if visited[parentDigest] {
				continue
			}
			visited[parentDigest] = true

			parentCert := d.getByDigestLocked(parentDigest)
			if parentCert != nil && parentCert.Round() > targetRound {
				stack = append(stack, parentCert)
			}
		}
	}

	return false
}
