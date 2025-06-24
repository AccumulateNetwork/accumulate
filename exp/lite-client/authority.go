package liteclient

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// AuthoritySet holds one or more public keys and their signature threshold.
type AuthoritySet struct {
	Keys      [][]byte
	Threshold uint64
}

// NewAuthoritySetFromAccount extracts signing keys from a KeyPage/KeyBook/ADI.
func NewAuthoritySetFromAccount(account interface{}) (*AuthoritySet, error) {
	switch acc := account.(type) {
	case *protocol.KeyPage:
		keys := make([][]byte, len(acc.Keys))
		for i, keySpec := range acc.Keys {
			keys[i] = keySpec.PublicKeyHash
		}
		return &AuthoritySet{Keys: keys, Threshold: acc.AcceptThreshold}, nil
	case *protocol.KeyBook:
		// Handle KeyBook case if needed
		return nil, fmt.Errorf("KeyBook handling not implemented")
	case *protocol.AccountAuth:
		return nil, fmt.Errorf("resolving authorities from URLs is not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported account type")
	}
}

// VerifySignatureAgainstAuthoritySet checks if a signature is valid for the given authority set.
// It returns true if the number of valid signatures meets the authority threshold.
func VerifySignatureAgainstAuthoritySet(
	signature protocol.KeySignature,
	hash []byte,
	authSet *AuthoritySet,
) (bool, error) {
	validCount := 0
	for _, pub := range authSet.Keys {
		valid, err := verifyEd25519Signature(signature, hash, pub)
		if err == nil && valid {
			validCount++
		}
	}
	if uint64(validCount) >= authSet.Threshold {
		return true, nil
	}
	return false, fmt.Errorf("not enough valid signatures: got %d, need %d", validCount, authSet.Threshold)
}

// verifyEd25519Signature checks a KeySignature against a public key and hash using ed25519.
func verifyEd25519Signature(signature protocol.KeySignature, hash []byte, publicKey []byte) (bool, error) {
	sigBytes := signature.GetSignature()
	if len(sigBytes) != ed25519.SignatureSize {
		return false, fmt.Errorf("unexpected signature length: got %d, want %d", len(sigBytes), ed25519.SignatureSize)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid public key length: got %d, want %d", len(publicKey), ed25519.PublicKeySize)
	}
	valid := ed25519.Verify(publicKey, hash, sigBytes)
	return valid, nil
}

// AuthorityTracker maintains a history of authority changes across major blocks.
type AuthorityTracker struct {
	history map[uint64]*AuthoritySet
}

// NewAuthorityTracker initializes a new AuthorityTracker.
func NewAuthorityTracker(initial *AuthoritySet, startBlock uint64) *AuthorityTracker {
	return &AuthorityTracker{
		history: map[uint64]*AuthoritySet{startBlock: initial},
	}
}

// GetAuthorityAt returns the AuthoritySet at a given block index.
func (t *AuthorityTracker) GetAuthorityAt(blockIndex uint64) (*AuthoritySet, error) {
	if auth, ok := t.history[blockIndex]; ok {
		return auth, nil
	}
	return nil, errors.New("authority set not found for block")
}

// RecordChange records an authority change at a specific block index.
func (t *AuthorityTracker) RecordChange(blockIndex uint64, newAuth *AuthoritySet) {
	t.history[blockIndex] = newAuth
}

// VerifyBlockSignatureAgainstAuthority checks if the signature is valid for the authority at the given block index.
// It returns true if the number of valid signatures meets the authority threshold.
func VerifyBlockSignatureAgainstAuthority(
	tracker *AuthorityTracker,
	blockIndex uint64,
	signature protocol.KeySignature,
	rootHash []byte,
) (bool, error) {
	authSet, err := tracker.GetAuthorityAt(blockIndex)
	if err != nil {
		return false, fmt.Errorf("could not get authority: %w", err)
	}
	return VerifySignatureAgainstAuthoritySet(signature, rootHash, authSet)
}
