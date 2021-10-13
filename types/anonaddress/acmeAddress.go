package types

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/protocol"
)

// GenerateAcmeAddress
// Given a public key, create a Ascii representation that can be used
// for the Anonymous token chain.
//
// Deprecated: use ./protocol.AnonymousAddress
func GenerateAcmeAddress(pubKey []byte) string {
	u, err := protocol.AnonymousAddress(pubKey, protocol.ACME)
	if err != nil {
		// If the hard-coded ACME token URL is invalid, something is seriously
		// wrong
		panic(fmt.Errorf("protocol is broken: %v", err))
	}

	return u.String()
}
