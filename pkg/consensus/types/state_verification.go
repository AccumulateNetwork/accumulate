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
// to enable cross-validation of state consistency. A message is for one
// partition, at one epoch, round and block, and its signature covers all of
// them: a node runs several partitions with one key, and its partitions number
// their rounds alike.
type StateHashMessage struct {
	// Partition is the partition whose state this is.
	Partition string
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

// stateHashDomain separates a state hash signature from every other message
// a validator key signs.
const stateHashDomain = "accumulate/dagbft/state-hash/v1"

// maxPartitionIDLen bounds the partition ID a message may carry.
const maxPartitionIDLen = 255

// NewStateHashMessage creates a new state hash message.
func NewStateHashMessage(partition string, round Round, epoch uint64, blockIndex uint64, stateHash StateHash, author ed25519.PublicKey) *StateHashMessage {
	return &StateHashMessage{
		Partition:  partition,
		Round:      round,
		Epoch:      epoch,
		BlockIndex: blockIndex,
		StateHash:  stateHash,
		Author:     author,
	}
}

// Position is where the message's state stands.
func (m *StateHashMessage) Position() StatePosition {
	return StatePosition{Epoch: m.Epoch, Round: m.Round, BlockIndex: m.BlockIndex}
}

// messageContent returns the bytes to be signed/verified: the domain, the
// partition, and the position and hash.
func (m *StateHashMessage) messageContent() []byte {
	data := make([]byte, 0, len(stateHashDomain)+1+len(m.Partition)+8+8+8+32)
	data = append(data, stateHashDomain...)
	data = append(data, byte(len(m.Partition)))
	data = append(data, m.Partition...)
	data = binary.BigEndian.AppendUint64(data, uint64(m.Round))
	data = binary.BigEndian.AppendUint64(data, m.Epoch)
	data = binary.BigEndian.AppendUint64(data, m.BlockIndex)
	data = append(data, m.StateHash[:]...)
	return data
}

// Sign signs the message with the given private key.
func (m *StateHashMessage) Sign(key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize {
		return errors.New("invalid private key size")
	}
	if len(m.Partition) > maxPartitionIDLen {
		return errors.New("partition ID too long")
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

	if len(m.Partition) > maxPartitionIDLen {
		return errors.New("partition ID too long")
	}

	content := m.messageContent()
	if !ed25519.Verify(m.Author, content, m.Signature) {
		return errors.New("invalid state hash message signature")
	}

	return nil
}

// Marshal serializes the state hash message.
func (m *StateHashMessage) Marshal() ([]byte, error) {
	if len(m.Partition) > maxPartitionIDLen {
		return nil, errors.New("partition ID too long")
	}
	// partitionLen(1) + partition + round(8) + epoch(8) + blockIndex(8) +
	// stateHash(32) + author(32) + sigLen(4) + sig
	data := make([]byte, 0, 1+len(m.Partition)+8+8+8+32+32+4+len(m.Signature))
	data = append(data, byte(len(m.Partition)))
	data = append(data, m.Partition...)
	data = binary.BigEndian.AppendUint64(data, uint64(m.Round))
	data = binary.BigEndian.AppendUint64(data, m.Epoch)
	data = binary.BigEndian.AppendUint64(data, m.BlockIndex)
	data = append(data, m.StateHash[:]...)
	author := make([]byte, 32)
	copy(author, m.Author)
	data = append(data, author...)
	data = binary.BigEndian.AppendUint32(data, uint32(len(m.Signature)))
	data = append(data, m.Signature...)
	return data, nil
}

// UnmarshalStateHashMessage deserializes a state hash message.
func UnmarshalStateHashMessage(data []byte) (*StateHashMessage, error) {
	if len(data) < 1 {
		return nil, errors.New("state hash message too short")
	}
	n := int(data[0])
	if len(data) < 1+n+8+8+8+32+32+4 {
		return nil, errors.New("state hash message too short")
	}

	m := &StateHashMessage{}
	offset := 1
	m.Partition = string(data[offset : offset+n])
	offset += n

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

// StatePosition is where a state stands: the epoch, the leader round that
// committed its block, and the block. Two reports describe the same state only
// at the same position.
type StatePosition struct {
	Epoch      uint64
	Round      Round
	BlockIndex uint64
}

// StateHashTracker tracks state hashes from validators for consistency
// verification. A node's state has diverged from its partition's at a
// position when validators holding a quorum of the stake of the committee of
// its round report one hash for that position and the node's own is another
// (consensus spec, "State hash comparison"). One validator's report, a split,
// or reports for another position decide nothing.
type StateHashTracker struct {
	mu sync.RWMutex

	// committeeAt is the committee of a round: who may report a hash for
	// it, and with what stake. Until it is set, remote hashes are not
	// recorded.
	committeeAt func(Round) *Committee

	// stateHashes maps position -> validator pubkey hex -> reported hash
	stateHashes map[StatePosition]map[string]remoteHash

	// localHashes maps round -> our local state hash and its position
	localHashes map[Round]localHash

	// maxTrackedRounds is the maximum number of rounds to track
	maxTrackedRounds int

	// divergenceCallbacks are called when state divergence is detected
	divergenceCallbacks []func(*StateDivergenceError)
}

// remoteHash is a hash a committee member reported.
type remoteHash struct {
	hash   StateHash
	author ed25519.PublicKey
}

// localHash is this node's hash and the position it is for.
type localHash struct {
	position StatePosition
	hash     StateHash
}

// NewStateHashTracker creates a new StateHashTracker.
func NewStateHashTracker(maxRounds int) *StateHashTracker {
	if maxRounds <= 0 {
		maxRounds = 100
	}
	return &StateHashTracker{
		stateHashes:      make(map[StatePosition]map[string]remoteHash),
		localHashes:      make(map[Round]localHash),
		maxTrackedRounds: maxRounds,
	}
}

// SetCommittee makes c the committee of every round.
func (t *StateHashTracker) SetCommittee(c *Committee) {
	if c == nil {
		t.setCommitteeAt(nil)
		return
	}
	t.setCommitteeAt(func(Round) *Committee { return c })
}

// SetSchedule counts the reports for a round in the committee the schedule
// gives that round, as headers, votes and certificates are checked (consensus
// spec, "Committee changes"). The schedule is read when a report is recorded
// and when a divergence is decided, so a change scheduled later applies to
// the rounds it takes over, and a validator not in a round's committee
// counts for nothing there.
func (t *StateHashTracker) SetSchedule(s *CommitteeSchedule) {
	if s == nil {
		t.setCommitteeAt(nil)
		return
	}
	t.setCommitteeAt(s.At)
}

func (t *StateHashTracker) setCommitteeAt(at func(Round) *Committee) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.committeeAt = at
}

// committeeLocked is the committee of round, or nil when none is set.
func (t *StateHashTracker) committeeLocked(round Round) *Committee {
	if t.committeeAt == nil {
		return nil
	}
	return t.committeeAt(round)
}

// OnDivergence registers a callback for state divergence events.
func (t *StateHashTracker) OnDivergence(callback func(*StateDivergenceError)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.divergenceCallbacks = append(t.divergenceCallbacks, callback)
}

// RecordLocalHash records our local state hash at a position.
func (t *StateHashTracker) RecordLocalHash(pos StatePosition, hash StateHash) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.localHashes[pos.Round] = localHash{position: pos, hash: hash}
	if err := t.divergenceLocked(pos.Round); err != nil {
		t.notifyDivergenceLocked(err)
	}
}

// RecordRemoteHash records a state hash a committee member reported for a
// position. Returns a StateDivergenceError if, with it, a quorum reports a
// hash other than our local one at our position. A hash from a key outside
// the committee of the position's round is not recorded.
func (t *StateHashTracker) RecordRemoteHash(pos StatePosition, validator ed25519.PublicKey, hash StateHash) *StateDivergenceError {
	t.mu.Lock()
	defer t.mu.Unlock()

	if c := t.committeeLocked(pos.Round); c == nil || c.StakeOf(validator) == 0 {
		return nil
	}

	// One report per member per round, its latest: a report at another
	// position for the round replaces the member's earlier one
	key := hexEncode(validator)
	for p, reports := range t.stateHashes {
		if p.Round != pos.Round || p == pos {
			continue
		}
		delete(reports, key)
		if len(reports) == 0 {
			delete(t.stateHashes, p)
		}
	}

	if t.stateHashes[pos] == nil {
		t.stateHashes[pos] = make(map[string]remoteHash)
	}
	t.stateHashes[pos][key] = remoteHash{hash: hash, author: validator}

	err := t.divergenceLocked(pos.Round)
	if err != nil {
		t.notifyDivergenceLocked(err)
	}
	return err
}

// divergenceLocked reports divergence at round if we have a local hash and
// validators holding a quorum of the stake of the round's committee report one
// other hash at the same position.
func (t *StateHashTracker) divergenceLocked(round Round) *StateDivergenceError {
	local, ok := t.localHashes[round]
	if !ok {
		return nil
	}
	committee := t.committeeLocked(round)
	if committee == nil {
		return nil
	}
	stake := map[StateHash]uint64{}
	for _, r := range t.stateHashes[local.position] {
		if r.hash == local.hash {
			continue
		}
		stake[r.hash] += committee.StakeOf(r.author)
		if committee.HasQuorum(stake[r.hash]) {
			return &StateDivergenceError{
				Round:          round,
				BlockIndex:     local.position.BlockIndex,
				ExpectedHash:   local.hash,
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
	local, ok := t.localHashes[round]
	return local.hash, ok
}

// GetRemoteHashes returns the remote state hashes reported at our position
// for a round, or at every position for the round if we have none.
func (t *StateHashTracker) GetRemoteHashes(round Round) map[string]StateHash {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]StateHash)
	for _, r := range t.reportsLocked(round) {
		for k, v := range r {
			result[k] = v.hash
		}
	}
	return result
}

// reportsLocked is the reports for a round: at our position if we have a
// local hash for it, otherwise at every position with that round.
func (t *StateHashTracker) reportsLocked(round Round) []map[string]remoteHash {
	if local, ok := t.localHashes[round]; ok {
		if r, ok := t.stateHashes[local.position]; ok {
			return []map[string]remoteHash{r}
		}
		return nil
	}
	var all []map[string]remoteHash
	for pos, r := range t.stateHashes {
		if pos.Round == round {
			all = append(all, r)
		}
	}
	return all
}

// CheckConsistency checks if all validators have the same state hash for a round.
// Returns true if consistent (or no data), false if any report differs.
func (t *StateHashTracker) CheckConsistency(round Round) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	local, hasLocal := t.localHashes[round]
	var first *StateHash
	if hasLocal {
		first = &local.hash
	}
	for _, r := range t.reportsLocked(round) {
		for _, v := range r {
			if first == nil {
				h := v.hash
				first = &h
				continue
			}
			if v.hash != *first {
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

	for pos := range t.stateHashes {
		if pos.Round < cutoff {
			delete(t.stateHashes, pos)
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

	seen := map[string]bool{}
	for _, r := range t.reportsLocked(round) {
		for k := range r {
			seen[k] = true
		}
	}
	return len(seen)
}
