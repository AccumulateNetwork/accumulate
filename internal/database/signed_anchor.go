package database

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SignedAnchorHash is the hash SystemData.SignedAnchor records for an anchor
// body: the hash of its encoding, so two bodies with the same hash are the
// same anchor.
func SignedAnchorHash(body protocol.TransactionBody) ([32]byte, error) {
	b, err := body.MarshalBinary()
	if err != nil {
		return [32]byte{}, errors.EncodingError.WithFormat("encode anchor body: %w", err)
	}
	return sha256.Sum256(b), nil
}
