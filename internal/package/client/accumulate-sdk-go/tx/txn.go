package txn

import (
	"github.com/AccumulateNetwork/accumulate/internal/url"

	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type Txn struct {
	Header *Header
	Body []byte `json:"body,omitempty" form:"body" query:"body" validate:"required"`
	txHash []byte
	TxBody *Envelope
	SigInfo []*transactions.SignatureInfo
}

type Header struct {
	Origin 			*url.URL `json:"origin,omitempty" form:"origin," query:"origin" validate:"required"`
	KeyPageHeight 	uint64 `json:"keypageheight,omitempty" form:"keypageheight," query:"keypageheight" validate:"required"`
	KeyPageIndex 	uint64 `json:"keypageindex,omitempty" form:"keypageindex," query:"keypageindex" validate:"required"`
	Nonce 			uint64 `json:"nonce,omitempty" form:"nonce," query:"nonce" validate:"required"`
}

type Envelope struct {
	Signatures []*transactions.ED25519Sig`json:"signatures,omitempty" form:"signatures," query:"signatures" validate:"required"`
	TxHash []byte `json:"txhash,omitempty" form:"txhash," query:"txhash" validate:"required"`
	Transaction Txn `json:"transaction,omitempty" form:"transaction," query:"transaction" validate:"required"`

}