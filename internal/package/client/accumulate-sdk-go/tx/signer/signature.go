package signing 

import (
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)
type SignatureV1 struct {
	PublicKey *transactions.ED25519Sig

	Data SignatureData
}

type SignatureDescriptor struct {
	Signature []*SignatureV1
}