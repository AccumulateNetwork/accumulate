// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"sync"
)

// StateHashMessage represents a state hash announcement from a validator.
// Validators gossip these messages after executing committed certificates
// to enable cross-validation of state consistency.
type StateHashMessage struct {
	// Round is the consensus round after which the state hash was computed.
	Round Round
	// Epoch is the epoch in which this message was created.
	Epoch uint64
	// BlockIndex is the block index corresponding to this state.
	BlockIndex uint64
	// StateHash is the hash of the state after executing the round.
	StateHash StateHash
	// Author is the public key of the validator who computed this hash.
	Author ed25519.PublicKey
	// Signature is the ed25519 signature over the message content.
	Signature []byte
}

// NewStateHashMessage creates a new state hash message.
func NewStateHashMessage(round Round, epoch uint64, blockIndex uint64, stateHash StateHash, author ed25519.PublicKey) *StateHashMessage {
	return &StateHashMessage{
		Round:      round,
		Epoch:      epoch,
		BlockIndex: blockIndex,
		StateHash:  stateHash,
		Author:     author,
	}
}

// messageContent returns the bytes to be signed/verified.
func (m *StateHashMessage) messageContent() []byte {
	// round(8) + epoch(8) + blockIndex(8) + stateHash(32)
	data := make([]byte, 8+8+8+32)
	offset := 0

	binary.BigEndian.PutUint64(data[offset:], uint64(m.Round))
	offset += 8

	binary.BigEndian.PutUint64(data[offset:], m.Epoch)
	offset += 8

	binary.BigEndian.PutUint64(data[offset:], m.BlockIndex)
	offset += 8

	copy(data[offset:], m.StateHash[:])

	return data
}

// Sign signs the message with the given private key.
func (m *StateHashMessage) Sign(key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize {
		return errors.New("invalid private key size")
	}

	content := m.messageContent()
	m.Signature = ed25519.Sign(key, content)
	return nil
}

// Verify verifies the message signature.
func (m *StateHashMessage) Verify() error {
	if len(m.Author) != ed25519.PublicKeySize {
		return errors.New("invalid author public key size")
	}

	if len(m.Signature) != ed25519.SignatureSize {
		return errors.New("invalid signature size")
	}

	content := m.messageContent()
	if !ed25519.Verify(m.Author, content, m.Signature) {
		return errors.New("invalid state hash message signature")
	}

	return nil
}

// Marshal serializes the state hash message.
func (m *StateHashMessage) Marshal() ([]byte, error) {
	// round(8) + epoch(8) + blockIndex(8) + stateHash(32) + author(32) + sigLen(4) + sig
	size := 8 + 8 + 8 + 32 + 32 + 4 + len(m.Signature)
	data := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint64(data[offset:], uint64(m.Round))
	offset += 8

	binary.BigEndian.PutUint64(data[offset:], m.Epoch)
	offset += 8

	binary.BigEndian.PutUint64(data[offset:], m.BlockIndex)
	offset += 8

	copy(data[offset:], m.StateHash[:])
	offset += 32

	copy(data[offset:], m.Author)
	offset += 32

	binary.BigEndian.PutUint32(data[offset:], uint32(len(m.Signature)))
	offset += 4

	copy(data[offset:], m.Signature)

	return data, nil
}

// UnmarshalStateHashMessage deserializes a state hash message.
func UnmarshalStateHashMessage(data []byte) (*StateHashMessage, error) {
	if len(data) < 92 { // 8 + 8 + 8 + 32 + 32 + 4
		return nil, errors.New("state hash message too short")
	}

	m := &StateHashMessage{}
	offset := 0

	m.Round = Round(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	m.Epoch = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	m.BlockIndex = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	copy(m.StateHash[:], data[offset:offset+32])
	offset += 32

	m.Author = make([]byte, 32)
	copy(m.Author, data[offset:offset+32])
	offset += 32

	sigLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if sigLen > 1024 {
		return nil, errors.New("signature too large")
	}

	if offset+int(sigLen) > len(data) {
		return nil, errors.New("state hash message truncated")
	}

	m.Signature = make([]byte, sigLen)
	copy(m.Signature, data[offset:offset+int(sigLen)])

	return m, nil
}

// StateDivergenceError represents a detected state divergence.
type StateDivergenceError struct {
	Round          Round
	BlockIndex     uint64
	ExpectedHash   StateHash
	ActualHash     StateHash
	ConflictAuthor ed25519.PublicKey
}

func (e *StateDivergenceError) Error() string {
	return "state divergence detected at round " + string(rune(e.Round))
}

// StateHashTracker tracks state hashes from validators for consistency
// verification. A node's state has diverged from its partition's at a round
// when a quorum of the committee reports one hash for the round and the
// node's own is another (consensus spec, "State hash comparison"). One
// validator's report, or a split, decides nothing.
type StateHashTracker struct {
	mu sync.RWMutex

	// committee is who may report a hash, and with what stake. Until it is
	// set, remote hashes are not recorded.
	committee *Committee

	// stateHashes maps round -> validator pubkey hex -> reported hash
	stateHashes map[Round]map[string]remoteHash

	// localHashes maps round -> our local state hash
	localHashes map[Round]StateHash

	// maxTrackedRounds is the maximum number of rounds to track
	maxTrackedRounds int

	// divergenceCallbacks are called when state divergence is detected
	divergenceCallbacks []func(*StateDivergenceError)
}

// remoteHash is a hash a committee member reported, with its stake.
type remoteHash struct {
	hash   StateHash
	author ed25519.PublicKey
	stake  uint64
}

// NewStateHashTracker creates a new StateHashTracker.
func NewStateHashTracker(maxRounds int) *StateHashTracker {
	if maxRounds <= 0 {
		maxRounds = 100
	}
	return &StateHashTracker{
		stateHashes:      make(map[Round]map[string]remoteHash),
		localHashes:      make(map[Round]StateHash),
		maxTrackedRounds: maxRounds,
	}
}

// SetCommittee sets the committee whose members' hashes are counted.
func (t *StateHashTracker) SetCommittee(c *Committee) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.committee = c
}

// OnDivergence registers a callback for state divergence events.
func (t *StateHashTracker) OnDivergence(callback func(*StateDivergenceError)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.divergenceCallbacks = append(t.divergenceCallbacks, callback)
}

// RecordLocalHash records our local state hash for a round.
func (t *StateHashTracker) RecordLocalHash(round Round, hash StateHash) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.localHashes[round] = hash
	if err := t.divergenceLocked(round); err != nil {
		t.notifyDivergenceLocked(err)
	}
}

// RecordRemoteHash records a state hash a committee member reported. Returns
// a StateDivergenceError if, with it, a quorum reports a hash other than our
// local one. A hash from a key outside the committee is not recorded.
func (t *StateHashTracker) RecordRemoteHash(round Round, validator ed25519.PublicKey, hash StateHash) *StateDivergenceError {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.committee == nil {
		return nil
	}
	stake := t.committee.StakeOf(validator)
	if stake == 0 {
		return nil
	}

	if t.stateHashes[round] == nil {
		t.stateHashes[round] = make(map[string]remoteHash)
	}
	t.stateHashes[round][hexEncode(validator)] = remoteHash{hash: hash, author: validator, stake: stake}

	err := t.divergenceLocked(round)
	if err != nil {
		t.notifyDivergenceLocked(err)
	}
	return err
}

// divergenceLocked reports divergence at round if we have a local hash and a
// quorum of the committee reports one other hash.
func (t *StateHashTracker) divergenceLocked(round Round) *StateDivergenceError {
	local, ok := t.localHashes[round]
	if !ok || t.committee == nil {
		return nil
	}
	stake := map[StateHash]uint64{}
	for _, r := range t.stateHashes[round] {
		if r.hash == local {
			continue
		}
		stake[r.hash] += r.stake
		if t.committee.HasQuorum(stake[r.hash]) {
			return &StateDivergenceError{
				Round:          round,
				ExpectedHash:   local,
				ActualHash:     r.hash,
				ConflictAuthor: r.author,
			}
		}
	}
	return nil
}

// notifyDivergenceLocked calls all divergence callbacks.
// Must be called with the lock held.
func (t *StateHashTracker) notifyDivergenceLocked(err *StateDivergenceError) {
	for _, cb := range t.divergenceCallbacks {
		go cb(err)
	}
}

// GetLocalHash returns the local state hash for a round, if recorded.
func (t *StateHashTracker) GetLocalHash(round Round) (StateHash, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	hash, ok := t.localHashes[round]
	return hash, ok
}

// GetRemoteHashes returns all remote state hashes for a round.
func (t *StateHashTracker) GetRemoteHashes(round Round) map[string]StateHash {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]StateHash)
	for k, v := range t.stateHashes[round] {
		result[k] = v.hash
	}
	return result
}

// CheckConsistency checks if all validators have the same state hash for a round.
// Returns true if consistent (or no data), false if divergence detected.
func (t *StateHashTracker) CheckConsistency(round Round) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	localHash, hasLocal := t.localHashes[round]
	remoteHashes, hasRemote := t.stateHashes[round]

	if !hasLocal && !hasRemote {
		return true // No data to check
	}

	// If we have local hash, check against all remotes
	if hasLocal && hasRemote {
		for _, remote := range remoteHashes {
			if remote.hash != localHash {
				return false
			}
		}
	}

	// Check all remote hashes against each other
	if hasRemote && len(remoteHashes) > 1 {
		var firstHash StateHash
		first := true
		for _, remote := range remoteHashes {
			if first {
				firstHash = remote.hash
				first = false
				continue
			}
			if remote.hash != firstHash {
				return false
			}
		}
	}

	return true
}

// Prune removes state hashes older than the given round minus maxTrackedRounds.
func (t *StateHashTracker) Prune(currentRound Round) {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := currentRound
	if cutoff > Round(t.maxTrackedRounds) {
		cutoff = currentRound - Round(t.maxTrackedRounds)
	} else {
		cutoff = 0
	}

	for round := range t.stateHashes {
		if round < cutoff {
			delete(t.stateHashes, round)
		}
	}

	for round := range t.localHashes {
		if round < cutoff {
			delete(t.localHashes, round)
		}
	}
}

// ValidatorHashCount returns the number of unique validators that have reported
// a state hash for the given round.
func (t *StateHashTracker) ValidatorHashCount(round Round) int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if roundHashes, ok := t.stateHashes[round]; ok {
		return len(roundHashes)
	}
	return 0
}
