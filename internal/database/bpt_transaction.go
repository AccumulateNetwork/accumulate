package database

import "fmt"

func (t *Transaction) loadState() (*transactionState, error) {
	state := new(transactionState)

	// Load main state
	var err error
	state.State, err = t.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("load transaction %X state: %w", t.hash()[:4], err)
	}

	// Load the transaction
	env, err := t.GetState()
	if err != nil {
		return nil, fmt.Errorf("load transaction %X: %w", t.hash()[:4], err)
	}
	state.Transaction = env.Transaction

	// Load signature sets
	state.Signatures = make([]*sigSetData, len(state.State.Signers))
	for i, signer := range state.State.Signers {
		state.Signatures[i], err = t.getSignatures(signer.GetUrl()).Get()
		if err != nil {
			return nil, fmt.Errorf("load transaction %X signers %s: %w", t.hash()[:4], signer.GetUrl(), err)
		}
	}

	return state, nil
}

func (t *Transaction) restoreState(s *transactionState) error {
	if len(s.State.Signers) != len(s.Signatures) {
		return fmt.Errorf("state is invalid: %d signers and %d signatures", len(s.State.Signers), len(s.Signatures))
	}

	err := t.PutState(&SigOrTxn{Transaction: s.Transaction})
	if err != nil {
		return fmt.Errorf("store state: %w", err)
	}

	err = t.PutStatus(s.State)
	if err != nil {
		return fmt.Errorf("store status: %w", err)
	}

	for i, set := range s.Signatures {
		signer := s.State.Signers[i].GetUrl()
		err = t.getSignatures(signer).Put(set)
		if err != nil {
			return fmt.Errorf("store signers %v: %w", signer, err)
		}
	}
	return nil
}
