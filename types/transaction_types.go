package types

import "fmt"

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
	TxTypeCreateSigSpec
	TxTypeCreateSigSpecGroup
	TxTypeAddCredits
	TxTypeUpdateKeyPage

	//The Following are only valid for DC & BVC use: any other source of this message will be rejected
	TxTypeSyntheticIdentityCreate = TxType(iota + 0x20)
	TxTypeSyntheticCreateChain
	TxTypeSyntheticTokenAccountCreate
	TxTypeSyntheticTokenDeposit
	TxTypeSyntheticTxResponse
	TxTypeSyntheticDepositCredits
	TxTypeBvcSubmission
	TxTypeSyntheticGenesis
	TxTypeDataStore //Data Store can only be sent and thus authorized by an authority node
	TxTypeAdminVote

	txTypeSyntheticBase = TxTypeSyntheticIdentityCreate
)

// Enum value map for TxType.
var TxTypeValue = map[string]TxType{}

func init() {
	all := []TxType{
		TxTypeUnknown,
		TxTypeIdentityCreate,
		TxTypeTokenAccountCreate,
		TxTypeTokenTx,
		TxTypeDataChainCreate,
		TxTypeDataEntry,
		TxTypeScratchChainCreate,
		TxTypeScratchEntry,
		TxTypeTokenCreate,
		TxTypeKeyUpdate,
		TxTypeMultisigTx,
		TxTypeStateQuery,
		TxTypeCreateSigSpec,
		TxTypeCreateSigSpec,
		TxTypeCreateSigSpecGroup,
		TxTypeAddCredits,
		TxTypeUpdateKeyPage,

		TxTypeSyntheticIdentityCreate,
		TxTypeSyntheticCreateChain,
		TxTypeSyntheticTokenAccountCreate,
		TxTypeSyntheticTokenDeposit,
		TxTypeSyntheticTxResponse,
		TxTypeSyntheticDepositCredits,
		TxTypeBvcSubmission,
		TxTypeSyntheticGenesis,
		TxTypeDataStore,
		TxTypeAdminVote,
	}
	for _, t := range all {
		TxTypeValue[t.Name()] = t
	}
}

//Name will return the name of the type
func (t TxType) Name() string {
	switch t {
	case TxTypeUnknown:
		return "unknown"
	case TxTypeIdentityCreate:
		return "identityCreate"
	case TxTypeTokenAccountCreate:
		return "tokenAccountCreate"
	case TxTypeTokenTx:
		return "tokenTx"
	case TxTypeDataChainCreate:
		return "dataChainCreate"
	case TxTypeDataEntry:
		return "dataEntry"
	case TxTypeScratchChainCreate:
		return "scratchChainCreate"
	case TxTypeScratchEntry:
		return "scratchEntry"
	case TxTypeTokenCreate:
		return "tokenCreate"
	case TxTypeKeyUpdate:
		return "keyUpdate"
	case TxTypeMultisigTx:
		return "multisigTx"
	case TxTypeStateQuery:
		return "stateQuery"
	case TxTypeCreateSigSpec:
		return "createSigSpec"
	case TxTypeCreateSigSpecGroup:
		return "createSigSpecGroup"
	case TxTypeAddCredits:
		return "addCredits"
	case TxTypeUpdateKeyPage:
		return "updateKeyPage"
	case TxTypeSyntheticIdentityCreate:
		return "syntheticIdentityCreate"
	case TxTypeSyntheticCreateChain:
		return "syntheticCreateChain"
	case TxTypeSyntheticTokenAccountCreate:
		return "syntheticTokenAccountCreate"
	case TxTypeSyntheticTokenDeposit:
		return "syntheticTokenDeposit"
	case TxTypeSyntheticTxResponse:
		return "syntheticTxResponse"
	case TxTypeSyntheticDepositCredits:
		return "syntheticDepositCredits"
	case TxTypeBvcSubmission:
		return "bvcSubmission"
	case TxTypeSyntheticGenesis:
		return "syntheticGenesis"
	case TxTypeDataStore:
		return "dataStore"
	case TxTypeAdminVote:
		return "adminVote"
	default:
		return fmt.Sprintf("TxType:%d", t)
	}
}

//SetType will set the type based on the string name submitted
func (t *TxType) SetType(s string) {
	*t = TxTypeValue[s]
}

//AsUint64 casts as a uint64
func (t TxType) AsUint64() uint64 {
	return uint64(t)
}

func (t TxType) String() string { return t.Name() }

func (t TxType) IsSynthetic() bool { return t >= txTypeSyntheticBase }

func (t *TxType) UnmarshalJSON(data []byte) error {
	var s String
	err := s.UnmarshalJSON(data)
	if err != nil {
		return fmt.Errorf("error unmarshaling transaction type")
	}
	t.SetType(*s.AsString())

	return nil
}

func (t *TxType) MarshalJSON() ([]byte, error) {
	s := String(t.String())
	return s.MarshalJSON()
}
