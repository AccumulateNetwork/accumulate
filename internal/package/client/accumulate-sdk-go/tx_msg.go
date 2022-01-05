package sdk

import (

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/internal/package/client/accumulate-sdk-go/crypto"
)

type (
	// Msg is the interface from the protocoltransaction that must be  fulfill.
	Msg interface {
		protocol.TransactionPayload

		GetSigners() []AccAddress
	}

	Txn interface {
		GetMsgs() []Msg

		ValidateBasic() error
	}


	// Signature interface for setting and return transaction signatures.
	Signature interface {
		GetPublicKey() crypto.PubKey
		GetSignature() []byte
	}
	
)