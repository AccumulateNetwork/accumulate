package synthetic

import (
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
)

type AckNakCode int

const (
	AckNakCode_Success AckNakCode = iota
	AckNakCode_Identity_Not_Found
	AckNakCode_Invalid_Chain_Id
	AckNakCode_State_Change_Fail
	AckNakCode_Transaction_Parsing_Error
	AckNakCode_Invalid_Signature
	AckNakCode_Sender_Not_Authorized
)

type TransactionAckNak struct {
	Txid             types.Bytes32   `json:"txid"`               // txid of original tx
	Code             AckNakCode      `json:"code"`               // Pass / Fail Return Code
	ReceiverIdentity types.Bytes32   `json:"receiver-id"`        // receiver's identity of transaction
	ReceiverChainId  types.Bytes32   `json:"receiver-chain-id"`  // receiver's chain id
	Metadata         json.RawMessage `json:"metadata,omitempty"` // Reason for Pass / Fail
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
	copy(tan.ReceiverIdentity[:], receiverid)
	copy(tan.ReceiverChainId[:], receiverchainid)
	if md != nil {
		copy(tan.Metadata, *md)
	}

	return tan
}
