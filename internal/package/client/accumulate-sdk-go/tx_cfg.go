package sdk

import signing "github.com/AccumulateNetwork/accumulate/internal/package/client/accumulate-sdk-go/tx/signer"

type (

	// TxBuilder to set txn, fee amnt token sending, generate signatures,
	TxnBuilder interface {
		GetTxn() Txn

		SetSignature(signatures ...signing.SignatureV1) error
		SetMsgs(msg ...Msg) error
	//	SetFeeAmount(amount uint64)
	}

	TxnConfig interface {
		NewTxnBuilder() TxnBuilder
	}
)