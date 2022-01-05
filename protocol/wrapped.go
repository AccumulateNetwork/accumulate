package protocol

import (
	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/types"
)

type WrappedTxPayload struct {
	TransactionPayload
}

func (w *WrappedTxPayload) UnmarshalBinary(data []byte) error {
	v, _ := common.BytesUint64(data)
	typ := types.TransactionType(v)

	pl, err := NewTransaction(typ)
	if err != nil {
		return err
	}

	err = pl.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	w.TransactionPayload = pl
	return nil
}
