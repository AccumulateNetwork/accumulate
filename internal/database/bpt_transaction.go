package database

import "fmt"

func (t *Transaction) state(full bool) (*transactionState, error) {
	state := new(transactionState)

	// Load main state
	var err error
	state.State, err = t.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("load transaction %X state: %w", t.id[:4], err)
	}

	// Do we need the full transaction state?
	if !full {
		return state, nil
	}

	// Load the transaction
	env, err := t.GetState()
	if err != nil {
		return nil, fmt.Errorf("load transaction %X: %w", t.id[:4], err)
	}
	state.Transaction = env.Transaction

	// Load signature sets
	state.Signatures = make([]*sigSetData, len(state.State.Signers))
	for i, signer := range state.State.Signers {
		state.Signatures[i], err = t.signatures(signer.GetUrl()).Get()
		if err != nil {
			return nil, fmt.Errorf("load transaction %X signers %s: %w", t.id[:4], signer.GetUrl(), err)
		}
	}

	return state, nil
}
