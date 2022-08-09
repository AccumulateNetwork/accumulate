package database

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func GetSignaturesForSigner(transaction *Transaction, signer protocol.Signer) ([]protocol.Signature, error) {
	// Load the signature set
	sigset, err := transaction.ReadSignaturesForSigner(signer)
	if err != nil {
		return nil, fmt.Errorf("load signatures set %v: %w", signer.GetUrl(), err)
	}

	entries := sigset.Entries()
	signatures := make([]protocol.Signature, 0, len(entries))
	for _, e := range entries {
		state, err := transaction.parent.Transaction(e.SignatureHash[:]).GetState()
		if err != nil {
			return nil, fmt.Errorf("load signature entry %X: %w", e.SignatureHash, err)
		}

		if state.Signature == nil {
			// This should not happen
			continue
		}

		signatures = append(signatures, state.Signature)
	}
	return signatures, nil
}
