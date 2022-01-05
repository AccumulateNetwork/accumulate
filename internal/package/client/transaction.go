package client

import (
	sdk "github.com/AccumulateNetwork/accumulate/internal/package/client/accumulate-sdk-go"
	"github.com/AccumulateNetwork/accumulate/internal/package/client/accumulate-sdk-go/crypto"
)

type (
	Generator interface {
		NewTxn() ClientTxn
		NewSig() ClientSigner
	}

	ClientTxn interface {
		sdk.Txn

	}	

	ClientSigner interface {
		sdk.Signature
		SetPublicKey(crypto.PubKey) error
		SetSignature([]byte)
	}
)