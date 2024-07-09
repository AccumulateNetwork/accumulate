package protocol

import (
	_ "embed"
	"encoding/json"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

func init() {
	//need to handle the edge cases for Key page operations and data entries
	encoding.RegisterEnumeratedTypeInterface(NewKeyPageOperation)
	encoding.RegisterEnumeratedTypeInterface(NewDataEntry)
}

func NewEip712TransactionDefinition(txn *Transaction) *encoding.TypeDefinition {
	txnSchema := &[]*encoding.TypeField{
		encoding.NewTypeField("header", "TransactionHeader"),
		encoding.NewTypeField("body", txn.Body.Type().String()),
	}

	return &encoding.TypeDefinition{txnSchema}
}

// MarshalEip712 This will create an EIP712 json message needed to submit to a wallet
func MarshalEip712(transaction Transaction) (ret []byte, err error) {
	type eip712 struct {
		PrimaryType  string `json:"primary_type"`
		Types        []encoding.TypeDefinition
		EIP712Domain encoding.EIP712Domain `json:"EIP712Domain"`
		Message      json.RawMessage       `json:"message"`
	}
	e := eip712{}
	e.PrimaryType = "Transaction"
	e.Message, err = transaction.MarshalJSON()
	e.EIP712Domain = encoding.Eip712Domain
	//go through transaction and build types list
	txMap := map[string]interface{}{}
	txj, err := transaction.MarshalJSON()
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(txj, &txMap)
	if err != nil {
		return nil, err
	}

	//capture types from txMap

	j, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return j, nil
} //

func Eip712Hasher(txn *Transaction) ([]byte, error) {
	j, err := txn.MarshalJSON()
	if err != nil {
		return nil, err
	}

	var jtx map[string]interface{}
	err = json.Unmarshal(j, &jtx)
	if err != nil {
		return nil, err
	}

	h, err := encoding.Eip712Hash(jtx, "Transaction", NewEip712TransactionDefinition(txn))
	if err != nil {
		return nil, err
	}

	return h, nil
}
