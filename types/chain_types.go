package types

import (
	"fmt"
)

type ChainType uint64

//ChainType enumeration order matters, do not change order or insert new enums.
const (
	ChainTypeUnknown              = ChainType(iota)
	ChainTypeDC                   // Directory Chain
	ChainTypeBVC                  // Block Validator Chain
	ChainTypeAdi                  // Accumulate Digital/Distributed Identity/Identifier/Domain
	ChainTypeToken                // Token Issue
	ChainTypeTokenAccount         // Token Account
	ChainTypeAnonTokenAccount     // Anonymous Token Account
	ChainTypeTransactionReference // Transaction Reference Chain
	ChainTypeTransaction          // Transaction Chain
	ChainTypePendingTransaction   // Pending Chain
	ChainTypeSigSpec              // Signature Specification chain
	ChainTypeSigSpecGroup         // Signature Specification Group chain
)

// Enum value maps for ChainType.
var (
	ChainTypeName = map[ChainType]string{
		ChainTypeUnknown:              "ChainTypeUnknown",
		ChainTypeDC:                   "ChainTypeDC",
		ChainTypeBVC:                  "ChainTypeBVC",
		ChainTypeAdi:                  "ChainTypeAdi",
		ChainTypeToken:                "ChainTypeToken",
		ChainTypeTokenAccount:         "ChainTypeTokenAccount",
		ChainTypeAnonTokenAccount:     "ChainTypeAnonTokenAccount",
		ChainTypeTransactionReference: "ChainTypeTransactionReference",
		ChainTypeTransaction:          "ChainTypeTransaction",
		ChainTypePendingTransaction:   "ChainTypePendingTransaction",
		ChainTypeSigSpec:              "ChainTypeSigSpec",
		ChainTypeSigSpecGroup:         "ChainTypeSigSpecGroup",
	}
	ChainTypeValue = map[string]ChainType{
		"ChainTypeUnknown":              ChainTypeUnknown,
		"ChainTypeDC":                   ChainTypeDC,
		"ChainTypeBVC":                  ChainTypeBVC,
		"ChainTypeAdi":                  ChainTypeAdi,
		"ChainTypeToken":                ChainTypeToken,
		"ChainTypeTokenAccount":         ChainTypeTokenAccount,
		"ChainTypeAnonTokenAccount":     ChainTypeAnonTokenAccount,
		"ChainTypeTransactionReference": ChainTypeTransactionReference,
		"ChainTypeTransaction":          ChainTypeTransaction,
		"ChainTypePendingTransaction":   ChainTypePendingTransaction,
		"ChainTypeSigSpec":              ChainTypeSigSpec,
		"ChainTypeSigSpecGroup":         ChainTypeSigSpecGroup,
	}
)

//Name will return the name of the type
func (t ChainType) Name() string {
	if name := ChainTypeName[t]; name != "" {
		return name
	}
	return ChainTypeUnknown.Name()
}

//SetType will set the type based on the string name submitted
func (t *ChainType) SetType(s string) {
	*t = ChainTypeValue[s]
}

//AsUint64 casts as a uint64
func (t ChainType) AsUint64() uint64 {
	return uint64(t)
}

func (t ChainType) String() string { return t.Name() }

func (t ChainType) IsTransaction() bool {
	switch t {
	case ChainTypeTransaction, ChainTypeTransactionReference, ChainTypePendingTransaction:
		return true
	default:
		return false
	}
}

func (t *ChainType) UnmarshalJSON(data []byte) error {
	var s String
	err := s.UnmarshalJSON(data)
	if err != nil {
		return fmt.Errorf("error unmarshaling chain type")
	}
	t.SetType(*s.AsString())

	return nil
}

func (t *ChainType) MarshalJSON() ([]byte, error) {
	s := String(t.String())
	return s.MarshalJSON()
}
