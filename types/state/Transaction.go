package state

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
)

type transactionHeader struct {
	KeyType   KeyType
	PublicKey types.Bytes `json:"publicKey" form:"publicKey" query:"publicKey" validate:"required"`
	Signature types.Bytes `json:"sig" form:"sig" query:"sig" validate:"required"`
	ChainId   types.Bytes
	KeyChain  *types.Bytes `json:"keyChain" form:"KeyGroup" query:"KeyGroup" validate:"required"`
	Status    string       `json:"status" form:"status" query:"status" validate:"required"`
}

//transaction object will either be on main chain or combined with the header and placed on pending chain.
type transaction struct {
	Transaction *types.Bytes `json:"tx,omitempty" form:"tx" query:"tx" validate:"optional"`
}

// Transaction can take several modes, the basic is the signature information,
// i.e. transaction header (signature, rcd, transactionid, chainid)
// the body of the transaction can also be stored for pending transactions.
type Transaction struct {
	Entry
	Chain
	transaction
}

type PendingTransaction struct {
	Entry
	Chain
	transactionHeader
	transaction
}

func NewTransaction() *Transaction {
	tx := &Transaction{}
	return tx
}

func NewPendingTransaction() *PendingTransaction {
	tx := &PendingTransaction{}
	return tx
}

func (is *Transaction) GetChainUrl() string {
	return is.Chain.GetChainUrl()
}

func (is *Transaction) GetType() *types.Bytes32 {
	return is.Chain.GetType()
}

func (t *Transaction) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	headerData, err := t.Chain.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to marshal chain header for transaction, %v", err)
	}

	buffer.Write(headerData)

	return buffer.Bytes(), nil
}

func (t *Transaction) UnmarshalBinary(data []byte) error {

	i := 0

	err := t.Chain.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("unable to unmarshal data for transaction, %v", err)
	}

	i += t.Chain.GetHeaderSize()

	if len(data) < i {
		return fmt.Errorf("unable to unmarshal raw transaction data")
	}

	return nil
}

func (t *PendingTransaction) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	headerData, err := t.Chain.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to marshal chain header for transaction, %v", err)
	}

	buffer.Write(headerData)
	buffer.WriteByte(byte(t.KeyType))
	buffer.Write(t.ChainId.Bytes())
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

func (t *PendingTransaction) UnmarshalBinary(data []byte) error {
	length := len(data)
	if length < 2*32+64+1 {
		return fmt.Errorf("insufficient data to unmarshal transaction state")
	}

	i := 0

	err := t.Chain.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("unable to unmarshal data for transaction, %v", err)
	}

	i += t.Chain.GetHeaderSize()

	t.KeyType = KeyType(data[i])
	i += copy(t.ChainId[:], data[i:i+32])
	i += copy(t.PublicKey[:], data[i:i+32])

	i += copy(t.Signature[:], data[i:i+64])

	if t.KeyType == KeyTypeChain {
		if length < i+32 {
			return fmt.Errorf("insufficient data to unmarshal keychain")
		}
		keyChain := types.Bytes(data[i : i+32])
		t.KeyChain = &keyChain
		i += 32
	}

	if length > i {
		tx := types.Bytes(data[i:])
		t.Transaction = &tx
	}

	return nil
}

func (is *PendingTransaction) GetChainUrl() string {
	return is.Chain.GetChainUrl()
}

func (is *PendingTransaction) GetType() *types.Bytes32 {
	return is.Chain.GetType()
}
