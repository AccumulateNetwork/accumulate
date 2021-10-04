package types

type TxType uint64

//Never change the order of these types.
//If any types are deleted use a placeholder to prevent index number from changing.
const (
	TxTypeUnknown = TxType(iota)
	TxTypeIdentityCreate
	TxTypeTokenAccountCreate
	TxTypeTokenTx
	TxTypeDataChainCreate
	TxTypeDataEntry //per 256 btes
	TxTypeScratchChainCreate
	TxTypeScratchEntry //per 256 bytes
	TxTypeTokenCreate
	TxTypeKeyUpdate //update keys on the keychain the identity
	TxTypeMultisigTx
	TxTypeStateQuery //sends a query to a chain and returns its state information

	//The Following are only valid for DC & BVC use: any other source of this message will be rejected
	TxTypeSyntheticIdentityCreate = TxType(iota + 0x20)
	TxTypeSyntheticTokenTx
	TxTypeSyntheticTokenDeposit
	TxTypeSyntheticTxResponse
	TxTypeBvcSubmission
	TxTypeStateStore //
	TxTypeDataStore  //Data Store can only be sent and thus authorized by an authority node
	TxTypeAdminVote
)

// Enum value maps for TxType.
var (
	TxTypeName = map[TxType]string{
		TxTypeUnknown:                 "Unknown",
		TxTypeIdentityCreate:          "identityCreate",
		TxTypeTokenAccountCreate:      "tokenAccountCreate",
		TxTypeTokenTx:                 "tokenTx",
		TxTypeDataChainCreate:         "dataChainCreate",
		TxTypeDataEntry:               "dataEntry",
		TxTypeScratchChainCreate:      "scratchChainCreate",
		TxTypeScratchEntry:            "scratchEntry",
		TxTypeTokenCreate:             "tokenCreate", //Token Issuance
		TxTypeKeyUpdate:               "keyUpdate",
		TxTypeStateQuery:              "stateQuery",
		TxTypeDataStore:               "dataStore",
		TxTypeStateStore:              "stateStore",
		TxTypeAdminVote:               "adminVote",
		TxTypeMultisigTx:              "multisigTx",
		TxTypeSyntheticIdentityCreate: "syntheticIdentityCreate",
		TxTypeSyntheticTokenTx:        "syntheticTokenTx",
		TxTypeSyntheticTokenDeposit:   "syntheticTokenDeposit",
		TxTypeSyntheticTxResponse:     "syntheticTxResponse",
		TxTypeBvcSubmission:           "bvcSubmission",
	}
	TxTypeValue = map[string]TxType{
		"Unknown":                 TxTypeUnknown,
		"identityCreate":          TxTypeIdentityCreate,
		"TokenAccountCreate":      TxTypeTokenAccountCreate,
		"TokenTx":                 TxTypeTokenTx,
		"dataChainCreate":         TxTypeDataChainCreate,
		"dataEntry":               TxTypeDataEntry,
		"scratchChainCreate":      TxTypeScratchChainCreate,
		"scratchEntry":            TxTypeScratchEntry,
		"tokenCreate":             TxTypeTokenCreate,
		"keyUpdate":               TxTypeKeyUpdate,
		"stateQuery":              TxTypeStateQuery,
		"dataStore":               TxTypeDataStore,
		"stateStore":              TxTypeStateStore,
		"adminVote":               TxTypeAdminVote,
		"multisigTx":              TxTypeMultisigTx,
		"syntheticIdentityCreate": TxTypeSyntheticIdentityCreate,
		"syntheticTokenTx":        TxTypeSyntheticTokenTx,
		"syntheticTokenDeposit":   TxTypeSyntheticTokenDeposit,
		"syntheticTxResponse":     TxTypeSyntheticTxResponse,
		"bvcSubmission":           TxTypeBvcSubmission,
	}
)

//Name will return the name of the type
func (t TxType) Name() string {
	if name := TxTypeName[t]; name != "" {
		return name
	}
	return TxTypeUnknown.Name()
}

//SetType will set the type based on the string name submitted
func (t *TxType) SetType(s string) {
	*t = TxTypeValue[s]
}

//AsUint64 casts as a uint64
func (t TxType) AsUint64() uint64 {
	return uint64(t)
}
