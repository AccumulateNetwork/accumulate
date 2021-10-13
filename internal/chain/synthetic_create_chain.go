package chain

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type SyntheticCreateChain struct{}

func (SyntheticCreateChain) Type() types.TxType {
	return types.TxTypeSyntheticCreateChain
}

type createChain struct {
	Url  *url.URL
	Data []byte
}

func willCreateChain(ccs []createChain, id []byte) bool {
	for _, cc := range ccs {
		if cc.Url == nil {
			continue
		}
		if bytes.Equal(cc.Url.ResourceChain(), id) {
			return true
		}
	}
	return false
}

func (SyntheticCreateChain) check(st *state.StateEntry, tx *transactions.GenTransaction) ([]createChain, error) {
	scc := new(protocol.SyntheticCreateChain)
	err := tx.As(scc)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	if scc.Cause == [32]byte{} {
		return nil, fmt.Errorf("cause is missing")
	}

	ccs := make([]createChain, len(scc.Chains))
	for i, cc := range scc.Chains {
		chain := new(state.ChainHeader)
		err := chain.UnmarshalBinary(cc)
		if err != nil {
			return nil, fmt.Errorf("invalid chain payload: %v", err)
		}
		// TODO Check that the chain can be unmarshalled into whatever type it
		// claims to be. It should be OK to skip this, since synthetic
		// transactions should already be validated.

		u, err := url.Parse(chain.GetChainUrl())
		if err != nil {
			return nil, fmt.Errorf("invalid chain URL: %v", err)
		}
		ccs[i].Url = u
		ccs[i].Data = cc

		if _, err := st.DB.GetCurrentEntry(u.ResourceChain()); err == nil {
			return nil, fmt.Errorf("chain %q already exists", u.String())
		} else if !errors.Is(err, state.ErrNotFound) {
			return nil, fmt.Errorf("error fetching %q: %v", u.String(), err)
		}

		var adi [32]byte
		copy(adi[:], u.IdentityChain())

		if bytes.Equal(u.ResourceChain(), adi[:]) {
			if chain.Type != types.ChainTypeAdi {
				return nil, fmt.Errorf("%v cannot be its own identity", chain.Type)
			}
			continue
		}

		if willCreateChain(ccs, adi[:]) {
			continue // Found identity
		}

		_, err = st.DB.GetCurrentEntry(adi[:])
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

func (SyntheticCreateChain) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	_, err := SyntheticCreateChain{}.check(st, tx)
	return err
}

func (SyntheticCreateChain) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	ccs, err := SyntheticCreateChain{}.check(st, tx)
	if err != nil {
		return nil, err
	}

	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()
	for _, cc := range ccs {
		chainId := types.Bytes(cc.Url.ResourceChain()).AsBytes32()
		st.DB.AddStateEntry(&chainId, &txHash, &state.Object{Entry: cc.Data})
	}
	return new(DeliverTxResult), nil
}
