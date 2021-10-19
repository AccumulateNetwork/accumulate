package chain

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type SyntheticCreateChain struct{}

func (SyntheticCreateChain) Type() types.TxType {
	return types.TxTypeSyntheticCreateChain
}

func willCreateChain(ccs []state.Chain, id []byte) bool {
	for _, cc := range ccs {
		if cc == nil {
			continue
		}
		u, err := cc.Header().ParseUrl()
		if err != nil {
			continue
		}
		if bytes.Equal(u.ResourceChain(), id) {
			return true
		}
	}
	return false
}

func checkSyntheticCreateChain(st *StateManager, tx *transactions.GenTransaction) ([]state.Chain, error) {
	scc := new(protocol.SyntheticCreateChain)
	err := tx.As(scc)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	if scc.Cause == [32]byte{} {
		return nil, fmt.Errorf("cause is missing")
	}

	ccs := make([]state.Chain, len(scc.Chains))
	for i, cc := range scc.Chains {
		record, err := unmarshalRecord(&state.Object{Entry: cc})
		if err != nil {
			return nil, fmt.Errorf("invalid chain payload: %v", err)
		}

		ccs[i] = record

		u, err := record.Header().ParseUrl()
		if err != nil {
			return nil, fmt.Errorf("invalid chain URL: %v", err)
		}

		if _, err := st.LoadUrl(u); err == nil {
			return nil, fmt.Errorf("chain %q already exists", u.String())
		} else if !errors.Is(err, state.ErrNotFound) {
			return nil, fmt.Errorf("error fetching %q: %v", u.String(), err)
		}

		if u.Identity().Equal(u) {
			if record.Header().Type != types.ChainTypeAdi {
				return nil, fmt.Errorf("%v cannot be its own identity", record.Header().Type)
			}
			continue
		}

		var adi [32]byte
		copy(adi[:], u.IdentityChain())
		if willCreateChain(ccs, adi[:]) {
			continue // Found identity
		}

		_, err = st.Load(adi)
		if err == nil {
			continue // Found identity
		}
		if errors.Is(err, state.ErrNotFound) {
			return nil, fmt.Errorf("missing identity for %s", u.String())
		}
		return nil, fmt.Errorf("error fetching %q: %v", u.String(), err)
	}

	return ccs, nil
}

func (SyntheticCreateChain) CheckTx(st *StateManager, tx *transactions.GenTransaction) error {
	_, err := checkSyntheticCreateChain(st, tx)
	return err
}

func (SyntheticCreateChain) DeliverTx(st *StateManager, tx *transactions.GenTransaction) error {
	ccs, err := checkSyntheticCreateChain(st, tx)
	if err != nil {
		return err
	}

	for _, cc := range ccs {
		st.Store(cc)
	}
	return nil
}
