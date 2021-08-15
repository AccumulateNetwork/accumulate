package synthetic

import (
	"encoding/json"
	"strings"
)

type AckNakCode int

const (
	AckNakCode_Success AckNakCode = iota
	AckNakCode_Error_Unknown
	AckNakCode_Identity_Not_Found
	AckNakCode_Invalid_Chain_Id
	AckNakCode_State_Change_Fail
	AckNakCode_Transaction_Parsing_Error
	AckNakCode_Invalid_Signature
	AckNakCode_Sender_Not_Authorized
)

type TransactionAckNak struct {
	Header
	Code     AckNakCode      `json:"code"`               // Pass / Fail Return Code
	Metadata json.RawMessage `json:"metadata,omitempty"` // Reason for Pass / Fail
}

func NewTransactionAckNak(code AckNakCode, txid []byte, receiverid []byte, receiverchainid []byte, md *json.RawMessage) *TransactionAckNak {
	tan := &TransactionAckNak{}
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
	copy(tan.SourceIdentity[:], receiverid)
	copy(tan.SourceChainId[:], receiverchainid)
	if md != nil {
		copy(tan.Metadata, *md)
	}

	return tan
}

func (k *AckNakCode) MarshalJSON() ([]byte, error) {
	var str string

	switch {
	case *k == AckNakCode_Success:
		str = "success"
	case *k == AckNakCode_Identity_Not_Found:
		str = "identity-not-found"
	case *k == AckNakCode_Invalid_Chain_Id:
		str = "invalid-chain-id"
	case *k == AckNakCode_State_Change_Fail:
		str = "state-change-fail"
	case *k == AckNakCode_Transaction_Parsing_Error:
		str = "transaction-parsing-error"
	case *k == AckNakCode_Invalid_Signature:
		str = "invalid-signature"
	case *k == AckNakCode_Sender_Not_Authorized:
		str = "sender-not-authorized"
	default:
		*k = AckNakCode_Error_Unknown
	}

	return json.Marshal(&str)
}

func (k *AckNakCode) UnmarshalJSON(b []byte) error {
	str := strings.Trim(string(b), `"`)

	switch {
	case str == "success":
		*k = AckNakCode_Success
	case str == "identity-not-found":
		*k = AckNakCode_Identity_Not_Found
	case str == "invalid-chain-id":
		*k = AckNakCode_Invalid_Chain_Id
	case str == "state-change-fail":
		*k = AckNakCode_State_Change_Fail
	case str == "transaction-parsing-error":
		*k = AckNakCode_Transaction_Parsing_Error
	case str == "invalid-signature":
		*k = AckNakCode_Invalid_Signature
	case str == "sender-not-authorized":
		*k = AckNakCode_Sender_Not_Authorized
	default:
		*k = AckNakCode_Error_Unknown
	}

	return nil
}
