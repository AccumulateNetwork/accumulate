package response

import (
	"github.com/AccumulateNetwork/accumulate/types"
)

type TxReference struct {
	TxId types.Bytes32 `json:"txid" form:"txid" query:"txid"`
}
