package liteclient

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SignerValidator encapsulates the logic for verifying block signatures.
type SignerValidator struct {
	// TODO: Add fields if needed (e.g. cached key pages, logger, etc.)
}

// ValidateBlockSignature verifies that a block was signed correctly.
func (s *SignerValidator) ValidateBlockSignature(block interface{}, summary *messaging.BlockSummary, page *protocol.KeyPage) error {
	env, ok := block.(*messaging.Envelope)
	if !ok {
		return fmt.Errorf("unsupported block type: %T", block)
	}
	if summary == nil {
		return fmt.Errorf("block summary is required")
	}
	if len(env.Signatures) == 0 {
		return fmt.Errorf("no signatures found in envelope")
	}
	validCount := 0
	for _, sig := range env.Signatures {
		keySig, ok := sig.(protocol.KeySignature)
		if !ok {
			continue // skip if not a KeySignature
		}
		for _, key := range page.Keys {
			if valid, _ := verifyEd25519Signature(keySig, summary.StateTreeHash[:], key.PublicKeyHash); valid {
				validCount++
				break
			}
		}
	}
	if uint64(validCount) < page.AcceptThreshold {
		return fmt.Errorf("insufficient valid signatures: got %d, need %d", validCount, page.AcceptThreshold)
	}
	return nil
}

// ExtractSignaturesFromEnvelope extracts protocol.Signature pointers from a messaging.Envelope.
// This replaces the previous logic that incorrectly expected signatures on MakeMajorBlock.
func (s *SignerValidator) ExtractSignaturesFromEnvelope(block interface{}) ([]protocol.Signature, error) {
	env, ok := block.(*messaging.Envelope)
	if !ok {
		return nil, fmt.Errorf("unsupported block type: %T", block)
	}
	var sigs []protocol.Signature
	for _, sig := range env.Signatures {
		sigs = append(sigs, sig)
	}
	if len(sigs) == 0 {
		return nil, fmt.Errorf("no protocol.Signature found in envelope")
	}
	return sigs, nil
}
