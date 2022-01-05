package txn

import (
	"fmt"

	sdk "github.com/AccumulateNetwork/accumulate/internal/package/client/accumulate-sdk-go"
)

type config struct{}

func (c config) NewTxnBuilder() sdk.TxnBuilder {
	return newBuilder()
}

func (c config) WrapTxnBuilder(newTxn sdk.Txn) (sdk.TxnBuilder, error) {
	newBuilder, ok := newTxn.(*wrapper)
	if !ok {
		return nil, fmt.Errorf("unexpected type %T", newTxn)
	}
	return newBuilder, nil
}