package synthetic

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AccumulateNetwork/accumulate/smt/common"

	"github.com/AccumulateNetwork/accumulate/types"
)

type NakCode int

const (
	NakCodeErrorUnknown NakCode = iota
	NakCodeIdentityNotFound
	NakCodeInvalidChainId
	NakCodeStateChangeFail
	NakCodeTransactionParsingError
	NakCodeInvalidSignature
	NakCodeSenderNotAuthorized
)

type TransactionNak struct {
	Header
	Code     NakCode         `json:"code"`               // Pass / Fail Return Code
	Metadata json.RawMessage `json:"metadata,omitempty"` // Reason for Pass / Fail
}

func NewTransactionNak(txid types.Bytes, from *types.String, to *types.String, code NakCode, md *json.RawMessage) *TransactionNak {
	tan := &TransactionNak{}
	tan.Code = code

	tan.SetHeader(txid, from, to)

	if md != nil {
		copy(tan.Metadata, *md)
	}

	return tan
}

func (t *TransactionNak) MarshalBinary() (data []byte, err error) {
	data = append(data, common.Uint64Bytes(types.TxSyntheticTxResponse.AsUint64())...)
	header, err := t.Header.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("cannot marshal transactionnak, %v", err)
	}
	data = append(data, header...)
	data = append(data, common.Uint64Bytes(uint64(t.Code))...)
	data = append(data, common.SliceBytes(t.Metadata)...)
	return data, nil
}

func (t *TransactionNak) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if rerr := recover(); rerr != nil {
			err = fmt.Errorf("error unmarshaling TransactionNak %v", rerr)
		}
	}()

	txType, data := common.BytesUint64(data)
	if txType != types.TxSyntheticTxResponse.AsUint64() {
		return fmt.Errorf("invalid transaction type, expecting %s received %s",
			types.TxSyntheticTxResponse.Name(), types.TxType(txType).Name())
	}

	err = t.Header.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("")
	}

	data = data[t.Header.Size():]

	code, data := common.BytesUint64(data)
	t.Code = NakCode(code)

	t.Metadata, _ = common.BytesSlice(data)

	return nil
}

func (k *NakCode) MarshalJSON() ([]byte, error) {
	var str string

	switch *k {
	case NakCodeIdentityNotFound:
		str = "identity-not-found"
	case NakCodeInvalidChainId:
		str = "invalid-chain-id"
	case NakCodeStateChangeFail:
		str = "state-change-fail"
	case NakCodeTransactionParsingError:
		str = "transaction-parsing-error"
	case NakCodeInvalidSignature:
		str = "invalid-signature"
	case NakCodeSenderNotAuthorized:
		str = "sender-not-authorized"
	default:
		str = "unknown"
	}

	return json.Marshal(&str)
}

func (k *NakCode) UnmarshalJSON(b []byte) error {
	str := strings.Trim(string(b), `"`)

	switch {
	case str == "identity-not-found":
		*k = NakCodeIdentityNotFound
	case str == "invalid-chain-id":
		*k = NakCodeInvalidChainId
	case str == "state-change-fail":
		*k = NakCodeStateChangeFail
	case str == "transaction-parsing-error":
		*k = NakCodeTransactionParsingError
	case str == "invalid-signature":
		*k = NakCodeInvalidSignature
	case str == "sender-not-authorized":
		*k = NakCodeSenderNotAuthorized
	default:
		*k = NakCodeErrorUnknown
	}

	return nil
}
