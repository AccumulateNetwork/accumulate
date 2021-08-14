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

func (k *AckNakCode) UnmarshalJSON(b []byte) error {
	str := strings.Trim(string(b), `"`)

	switch {
	case str == "AckNakCode_Success":
		*k = AckNakCode_Success
	case str == "AckNakCode_Identity_Not_Found":
		*k = AckNakCode_Identity_Not_Found
	case str == "AckNakCode_Invalid_Chain_Id":
		*k = AckNakCode_Invalid_Chain_Id
	case str == "AckNakCode_State_Change_Fail":
		*k = AckNakCode_State_Change_Fail
	case str == "AckNakCode_Transaction_Parsing_Error":
		*k = AckNakCode_Transaction_Parsing_Error
	case str == "AckNakCode_Invalid_Signature":
		*k = AckNakCode_Invalid_Signature
	case str == "AckNakCode_Sender_Not_Authorized":
		*k = AckNakCode_Sender_Not_Authorized
	default:
		*k = AckNakCode_Error_Unknown
	}

	return nil
}
