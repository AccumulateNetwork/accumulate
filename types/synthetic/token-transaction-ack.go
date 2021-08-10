package synthetic

import (
	"encoding/json"
)


type AckNakCode int
const (
	NakCode_Success AckNakCode = iota
	NakCode_Identity_Not_Found
	NakCode_Invalid_Chain_Id
	NakCode_State_Change_Fail
	NakCode_Transaction_Parsing_Error
	NakCode_Invalid_Signature
	NakCode_Sender_Not_Authorized
)

type TokenTransactionAckNak struct {
	Txid           [32]byte        `json:"txid"`                 // txid of original tx
	Code           AckNakCode         `json:"code"`              // Pass / Fail Return Code
	ReceiverIdentity [32]byte        `json:"receiver-id"`        // receiver's identity of transaction
	ReceiverChainId  [32]byte        `json:"receiver-chain-id"`  // receiver's chain id
	Metadata       json.RawMessage `json:"metadata,omitempty"`   // Reason for Pass / Fail
}
