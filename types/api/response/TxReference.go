package response

import (
	"github.com/AccumulateNetwork/accumulated/types"
)

type TxReference struct {
	TxId types.Bytes32 `json:"txid" form:"txid" query:"txid"`
}
