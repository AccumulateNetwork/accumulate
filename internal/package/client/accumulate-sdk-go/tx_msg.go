package sdk

import (

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/internal/package/client/accumulate-sdk-go/crypto"
)

type (
	Msg interface {
		protocol.TransactionPayload

		Route() string

		Type() string

		GetSigners() []AccAddress
	}

	Txn interface {
		GetMsgs() []Msg

		ValidateBasic() error
	}

	Signature interface {
		GetPublicKey() crypto.PubKey
		GetSignature() []byte
	}
	
)