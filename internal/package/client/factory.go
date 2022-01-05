package client

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	sdk "github.com/AccumulateNetwork/accumulate/internal/package/client/accumulate-sdk-go"
	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type (
	// Factory is the interface that wraps the New method.
	Factory struct {
		adi string
		chainId []byte
		keyManager sdk.KeyManager
		txConfig sdk.TxnConfig
		query QueryData
	}

	QueryData func(string, []byte) ([]byte, int64, error)
)

func NewFactory() *Factory {
	return &Factory{}
}


func (f *Factory) ChainId() []byte {
	return f.chainId
}

func (f *Factory) ADI() string {
	return f.adi
}


func (f *Factory) KeyManager() sdk.KeyManager {
	return f.keyManager
}

func (f *Factory) BuildUnsigned(msgs []sdk.Msg) (sdk.TxnBuilder, error) {
	if f.chainId == nil {
		return nil, fmt.Errorf("chainId is nil")
	}
	tx := f.txConfig.NewTxnBuilder()

	if err := tx.SetMsgs(msgs...); err != nil {
		return nil, err
	}
	return tx, nil
}

// func (f *Factory) BuildSign() {}

func (f *Factory) Sign(nonce uint64, privateKey []byte, hash []byte)  error {
	e := new(transactions.ED25519Sig)
	e.Nonce = nonce
	nonceHash := append(common.Uint64Bytes(nonce), hash...)
	sig := ed25519.Sign(privateKey, nonceHash)
	
	e.PublicKey = append([]byte{}, privateKey[32:]...)
	if !bytes.Equal(privateKey, e.PublicKey) {
		return fmt.Errorf("Signature does not match")
	}
	e.Signature = sig

	return nil
}