package api

import (
	"encoding/json"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
)

// API Request Support Structure

// Signer holds the ADI and public key to use to verify the transaction
type Signer struct {
	URL       types.String  `json:"url" form:"url" query:"url" validate:"required"`
	PublicKey types.Bytes32 `json:"publicKey" form:"publicKey" query:"publicKey" validate:"required"`
}

// APIRequestRaw will leave the data payload intact which is required for signature verification
type APIRequestRaw struct {
	Tx   *APIRequestRawTx `json:"tx" form:"tx" query:"tx" validate:"required"`
	Sig  types.Bytes64    `json:"sig" form:"sig" query:"sig" validate:"required"`
	Wait bool             `json:"wait" form:"wait" query:"wait"`
}

// APIRequestRawTx is used to maintain the integrety of the Data field when it is read in
// The data field is used to verify the signature.  The transaction ledger is the
// concatenation of ( sha256(Signer.URL) | Data | Timestamp ).  The txid is the sha256(ledger)
// and the signature is ed25519( ledger )
type APIRequestRawTx struct {
	Data      *json.RawMessage `json:"data" form:"data" query:"data" validate:"required"`
	Signer    *Signer          `json:"signer" form:"signer" query:"signer" validate:"required"`
	Timestamp int64            `json:"timestamp" form:"timestamp" query:"timestamp" validate:"required"`
}

// APIRequestURL is used to unmarshal URL param into API methods, that retrieves data by URL
type APIRequestURL struct {
	URL  types.String `json:"url" form:"url" query:"url" validate:"required"`
	Wait bool         `json:"wait" form:"wait" query:"wait"`
}

// APIDataResponse is used in "get" API method response
type APIDataResponse struct {
	Type types.String     `json:"type" form:"type" query:"type" validate:"oneof:adi,token,tokenAccount,tokenTx,sigSpec,sigSpecGroup,assignSigSpec,addCredits"`
	Data *json.RawMessage `json:"data" form:"data" query:"data"`
}

// NewAPIRequest will convert create general transaction which is used inside of Accumulate and wraps a transaction type
func NewAPIRequest(sig *types.Bytes64, signer *Signer, nonce uint64, data []byte) (*transactions.GenTransaction, error) {

	gtx := new(transactions.GenTransaction)
	gtx.Routing = types.GetAddressFromIdentity(signer.URL.AsString())
	gtx.ChainID = types.GetChainIdFromChainPath(signer.URL.AsString())[:]
	gtx.Transaction = data

	gtx.SigInfo = new(transactions.SignatureInfo)
	gtx.SigInfo.Unused2 = nonce
	gtx.SigInfo.URL = *signer.URL.AsString()
	gtx.SigInfo.MSHeight = 0
	gtx.SigInfo.PriorityIdx = 0

	ed := new(transactions.ED25519Sig)
	ed.Nonce = gtx.SigInfo.Unused2
	ed.PublicKey = signer.PublicKey[:]
	ed.Signature = sig.Bytes()

	gtx.Signature = append(gtx.Signature, ed)

	return gtx, nil
}
