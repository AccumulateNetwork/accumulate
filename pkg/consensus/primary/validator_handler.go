// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"crypto/ed25519"
	"log/slog"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// ValidatorUpdateHandler handles validator set changes from the execution layer.
// It listens for validator updates from governance and applies them at appropriate
// round boundaries to maintain consensus safety.
type ValidatorUpdateHandler struct {
	primary *Primary

	mu             sync.Mutex
	pendingUpdates []types.ValidatorUpdate
	// applyAtRound specifies the round at which pending updates should be applied.
	// Updates are applied after this round completes to ensure all nodes transition together.
	applyAtRound types.Round
	// updateScheduled indicates whether there are pending updates scheduled.
	updateScheduled bool
}

// NewValidatorUpdateHandler creates a new ValidatorUpdateHandler.
func NewValidatorUpdateHandler(primary *Primary) *ValidatorUpdateHandler {
	return &ValidatorUpdateHandler{
		primary: primary,
	}
}

// OnValidatorSetChange is called by the execution layer when the validator set changes.
// This implements the adapter.ValidatorSetProvider callback interface.
// Updates are queued and applied at the next appropriate round boundary.
func (h *ValidatorUpdateHandler) OnValidatorSetChange(validators []adapter.ValidatorInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Convert adapter.ValidatorInfo to types.ValidatorUpdate by comparing with current committee
	currentCommittee := h.primary.Committee()
	updates := h.computeUpdates(currentCommittee, validators)

	if len(updates) == 0 {
		slog.Debug("No validator changes detected")
		return
	}

	// Schedule updates to be applied after the current round completes
	h.pendingUpdates = updates
	h.applyAtRound = h.primary.CurrentRound()
	h.updateScheduled = true

	slog.Info("Validator updates scheduled",
		"updateCount", len(updates),
		"applyAfterRound", h.applyAtRound)
}

// computeUpdates computes the set of updates needed to transform the current committee
// to match the new validator set from governance.
func (h *ValidatorUpdateHandler) computeUpdates(current *types.Committee, newValidators []adapter.ValidatorInfo) []types.ValidatorUpdate {
	// Build map of new validators
	newMap := make(map[string]adapter.ValidatorInfo, len(newValidators))
	for _, v := range newValidators {
		if v.Active {
			newMap[string(v.PublicKey[:])] = v
		}
	}

	var updates []types.ValidatorUpdate

	// Check current validators against new set
	for _, v := range current.Validators {
		keyStr := string(v.PublicKey)
		var key32 [32]byte
		copy(key32[:], v.PublicKey)

		if newV, exists := newMap[keyStr]; exists {
			// Validator exists in both - check for stake change
			if v.Stake != newV.Stake {
				updates = append(updates, types.ValidatorUpdate{
					PublicKey: v.PublicKey,
					Type:      types.ValidatorUpdateStake,
					NewStake:  newV.Stake,
				})
			}
			// Remove from newMap so we can find additions later
			delete(newMap, keyStr)
		} else {
			// Validator removed
			updates = append(updates, types.ValidatorUpdate{
				PublicKey: v.PublicKey,
				Type:      types.ValidatorUpdateRemove,
			})
		}
	}

	// Remaining entries in newMap are additions
	for _, v := range newMap {
		pubKey := make(ed25519.PublicKey, ed25519.PublicKeySize)
		copy(pubKey, v.PublicKey[:])
		updates = append(updates, types.ValidatorUpdate{
			PublicKey: pubKey,
			Type:      types.ValidatorUpdateAdd,
			NewStake:  v.Stake,
		})
	}

	return updates
}

// CheckAndApplyUpdates checks if there are pending updates and applies them
// if the current round has advanced past the scheduled round.
// This should be called periodically (e.g., after each round advancement).
func (h *ValidatorUpdateHandler) CheckAndApplyUpdates() {
	h.mu.Lock()
	if !h.updateScheduled {
		h.mu.Unlock()
		return
	}

	currentRound := h.primary.CurrentRound()
	if currentRound <= h.applyAtRound {
		h.mu.Unlock()
		return
	}

	// Copy updates and clear pending state while holding lock
	updates := h.pendingUpdates
	h.pendingUpdates = nil
	h.updateScheduled = false
	h.mu.Unlock()

	// Apply the epoch transition
	slog.Info("Applying scheduled validator updates",
		"updateCount", len(updates),
		"scheduledRound", h.applyAtRound,
		"currentRound", currentRound)

	// Don't reset round - continue from current round in new epoch
	h.primary.TransitionEpoch(updates, false)
}

// AddValidator queues a validator addition for the next epoch transition.
func (h *ValidatorUpdateHandler) AddValidator(pubKey ed25519.PublicKey, stake uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.pendingUpdates = append(h.pendingUpdates, types.ValidatorUpdate{
		PublicKey: pubKey,
		Type:      types.ValidatorUpdateAdd,
		NewStake:  stake,
	})

	if !h.updateScheduled {
		h.applyAtRound = h.primary.CurrentRound()
		h.updateScheduled = true
	}

	slog.Info("Validator addition queued",
		"pubkey", hexEncode(pubKey),
		"stake", stake)
}

// RemoveValidator queues a validator removal for the next epoch transition.
func (h *ValidatorUpdateHandler) RemoveValidator(pubKey ed25519.PublicKey) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.pendingUpdates = append(h.pendingUpdates, types.ValidatorUpdate{
		PublicKey: pubKey,
		Type:      types.ValidatorUpdateRemove,
	})

	if !h.updateScheduled {
		h.applyAtRound = h.primary.CurrentRound()
		h.updateScheduled = true
	}

	slog.Info("Validator removal queued",
		"pubkey", hexEncode(pubKey))
}

// UpdateStake queues a stake change for the next epoch transition.
func (h *ValidatorUpdateHandler) UpdateStake(pubKey ed25519.PublicKey, newStake uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.pendingUpdates = append(h.pendingUpdates, types.ValidatorUpdate{
		PublicKey: pubKey,
		Type:      types.ValidatorUpdateStake,
		NewStake:  newStake,
	})

	if !h.updateScheduled {
		h.applyAtRound = h.primary.CurrentRound()
		h.updateScheduled = true
	}

	slog.Info("Validator stake update queued",
		"pubkey", hexEncode(pubKey),
		"newStake", newStake)
}

// HasPendingUpdates returns true if there are pending validator updates.
func (h *ValidatorUpdateHandler) HasPendingUpdates() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.updateScheduled
}

// PendingUpdateCount returns the number of pending validator updates.
func (h *ValidatorUpdateHandler) PendingUpdateCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.pendingUpdates)
}

// ApplyAtRound returns the round after which pending updates will be applied.
func (h *ValidatorUpdateHandler) ApplyAtRound() types.Round {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.applyAtRound
}
