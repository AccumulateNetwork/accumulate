package state

import (
	"bytes"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
)

type transaction struct {
	Type        KeyType
	PublicKey   types.Bytes32  `json:"publicKey" form:"publicKey" query:"publicKey" validate:"required"`
	Signature   types.Bytes32  `json:"sig" form:"sig" query:"sig" validate:"required"`
	KeyChain    *types.Bytes32 `json:"keyChain" form:"KeyGroup" query:"KeyGroup" validate:"required"`
	Transaction *types.Bytes   `json:"tx,omitempty" form:"tx" query:"tx" validate:"optional"`
}

// Transaction can take several modes, the basic is the signature information,
// i.e. transaction header (signature, rcd, transactionid, chainid)
// the body of the transaction can also be stored for pending transactions.
type Transaction struct {
	Entry
	transaction
}

func NewTransaction(keyType KeyType, pubKey types.Bytes) *Transaction {
	tx := &Transaction{}
	return tx
}

func (t *Transaction) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.WriteByte(byte(t.Type))
	buffer.Write(t.PublicKey.Bytes())
	buffer.Write(t.Signature.Bytes())

	if t.KeyChain != nil {
		buffer.Write(t.KeyChain.Bytes())
	}

	if t.Transaction != nil {
		buffer.Write(t.Transaction.Bytes())
	}

	return buffer.Bytes(), nil
}

func (t *Transaction) UnmarshalBinary(data []byte) error {
	length := len(data)
	if length < 32*64+1 {
		return fmt.Errorf("insufficient data to unmarshal transaction state")
	}

	i := 0
	t.Type = KeyType(data[i])
	i += copy(t.PublicKey[:], data[i:i+32])

	i += copy(t.Signature[:], data[i:i+64])

	if t.Type == KeyTypeChain {
		if length < i+32 {
			return fmt.Errorf("insufficient data to unmarshal keychain")
		}
		keyChain := &types.Bytes32{}
		i += copy(keyChain[:], data[i:i+32])
		t.KeyChain = keyChain
	}

	if length > i {
		tx := types.Bytes{}
		tx = data[i:]
		t.Transaction = &tx
	}

	return nil
}
