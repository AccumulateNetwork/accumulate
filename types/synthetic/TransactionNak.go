package synthetic

import (
	"encoding/json"
	"strings"
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

func NewTransactionNak(code NakCode, txid []byte, receiverid []byte, receiverchainid []byte, md *json.RawMessage) *TransactionNak {
	tan := &TransactionNak{}
	tan.Code = code
	if len(txid) != 32 {
		return nil
	}
	if len(receiverid) != 32 {
		return nil
	}
	if len(receiverchainid) != 32 {
		return nil
	}

	copy(tan.Txid[:], txid)
	copy(tan.SourceAdiChain[:], receiverid)
	copy(tan.SourceChainId[:], receiverchainid)
	if md != nil {
		copy(tan.Metadata, *md)
	}

	return tan
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
