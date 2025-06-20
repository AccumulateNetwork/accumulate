package liteclient

import (
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SignerValidator encapsulates the logic for verifying block signatures.
type SignerValidator struct {
	// TODO: Add fields if needed (e.g. cached key pages, logger, etc.)
}

// ValidateBlockSignature verifies that a block was signed correctly.
func (s *SignerValidator) ValidateBlockSignature(block interface{}, page *protocol.KeyPage) error {
	// TODO: Implement block signature verification using keys from page.
	return nil
}

// ExtractSignaturesFromBlock retrieves the signature set used in a major block.
func (s *SignerValidator) ExtractSignaturesFromBlock(block interface{}) ([]*protocol.Signature, error) {
	// TODO: Parse block and extract all relevant signatures.
	return nil, nil
}
