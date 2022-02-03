package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/types"
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
