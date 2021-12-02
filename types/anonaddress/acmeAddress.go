package types

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
)

// GenerateAcmeAddress
// Given a public key, create a Ascii representation that can be used
// for the lite token chain.
//
// Deprecated: use ./protocol.LiteAddress
func GenerateAcmeAddress(pubKey []byte) string {
	u, err := protocol.LiteAddress(pubKey, protocol.ACME)
	if err != nil {
		// If the hard-coded ACME token URL is invalid, something is seriously
		// wrong
		panic(fmt.Errorf("protocol is broken: %v", err))
	}

	return u.String()
}
