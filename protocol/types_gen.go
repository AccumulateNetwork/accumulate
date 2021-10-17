package protocol

// GENERATED BY go run ./internal/cmd/genmarshal. DO NOT EDIT.

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type AddCredits struct {
	Recipient string `json:"recipient" form:"recipient" query:"recipient" validate:"required"`
	Amount    uint64 `json:"amount" form:"amount" query:"amount" validate:"required"`
}

type AnonTokenAccount struct {
	state.ChainHeader
	TokenUrl      string  `json:"tokenUrl" form:"tokenUrl" query:"tokenUrl" validate:"required"`
	Balance       big.Int `json:"balance" form:"balance" query:"balance" validate:"required"`
	TxCount       uint64  `json:"txCount" form:"txCount" query:"txCount" validate:"required"`
	Nonce         uint64  `json:"nonce" form:"nonce" query:"nonce" validate:"required"`
	CreditBalance big.Int `json:"creditBalance" form:"creditBalance" query:"creditBalance" validate:"required"`
}

type AssignSigSpecGroup struct {
	Url string `json:"url" form:"url" query:"url" validate:"required"`
}

type ChainParams struct {
	Url  string `json:"url" form:"url" query:"url" validate:"required"`
	Data []byte `json:"data" form:"data" query:"data" validate:"required"`
}

type CreateSigSpec struct {
	Url  string           `json:"url" form:"url" query:"url" validate:"required"`
	Keys []*KeySpecParams `json:"keys" form:"keys" query:"keys" validate:"required"`
}

type CreateSigSpecGroup struct {
	Url      string     `json:"url" form:"url" query:"url" validate:"required"`
	SigSpecs [][32]byte `json:"sigSpecs" form:"sigSpecs" query:"sigSpecs" validate:"required"`
}

type KeySpec struct {
	KeyAlgorithm  KeyAlgorithm  `json:"keyAlgorithm" form:"keyAlgorithm" query:"keyAlgorithm" validate:"required"`
	HashAlgorithm HashAlgorithm `json:"hashAlgorithm" form:"hashAlgorithm" query:"hashAlgorithm" validate:"required"`
	PublicKey     []byte        `json:"publicKey" form:"publicKey" query:"publicKey" validate:"required"`
	Nonce         uint64        `json:"nonce" form:"nonce" query:"nonce" validate:"required"`
}

type KeySpecParams struct {
	KeyAlgorithm  KeyAlgorithm  `json:"keyAlgorithm" form:"keyAlgorithm" query:"keyAlgorithm" validate:"required"`
	HashAlgorithm HashAlgorithm `json:"hashAlgorithm" form:"hashAlgorithm" query:"hashAlgorithm" validate:"required"`
	PublicKey     []byte        `json:"publicKey" form:"publicKey" query:"publicKey" validate:"required"`
}

type SigSpec struct {
	state.ChainHeader
	CreditBalance big.Int    `json:"creditBalance" form:"creditBalance" query:"creditBalance" validate:"required"`
	Keys          []*KeySpec `json:"keys" form:"keys" query:"keys" validate:"required"`
}

type SigSpecGroup struct {
	state.ChainHeader
	SigSpecs [][32]byte `json:"sigSpecs" form:"sigSpecs" query:"sigSpecs" validate:"required"`
}

type SyntheticCreateChain struct {
	Cause  [32]byte `json:"cause" form:"cause" query:"cause" validate:"required"`
	Chains [][]byte `json:"chains" form:"chains" query:"chains" validate:"required"`
}

type SyntheticDepositCredits struct {
	Cause  [32]byte `json:"cause" form:"cause" query:"cause" validate:"required"`
	Amount uint64   `json:"amount" form:"amount" query:"amount" validate:"required"`
}

type TxResult struct {
	SyntheticTxs []*TxSynthRef `json:"syntheticTxs" form:"syntheticTxs" query:"syntheticTxs" validate:"required"`
}

type TxSynthRef struct {
	Type  uint64   `json:"type" form:"type" query:"type" validate:"required"`
	Hash  [32]byte `json:"hash" form:"hash" query:"hash" validate:"required"`
	Url   string   `json:"url" form:"url" query:"url" validate:"required"`
	TxRef [32]byte `json:"txRef" form:"txRef" query:"txRef" validate:"required"`
}

func NewAnonTokenAccount() *AnonTokenAccount {
	v := new(AnonTokenAccount)
	v.Type = types.ChainTypeAnonTokenAccount
	return v
}

func NewSigSpec() *SigSpec {
	v := new(SigSpec)
	v.Type = types.ChainTypeSigSpec
	return v
}

func NewSigSpecGroup() *SigSpecGroup {
	v := new(SigSpecGroup)
	v.Type = types.ChainTypeSigSpecGroup
	return v
}

func (*AddCredits) GetType() types.TxType { return types.TxTypeAddCredits }

func (*AssignSigSpecGroup) GetType() types.TxType { return types.TxTypeAssignSigSpecGroup }

func (*CreateSigSpec) GetType() types.TxType { return types.TxTypeCreateSigSpec }

func (*CreateSigSpecGroup) GetType() types.TxType { return types.TxTypeCreateSigSpecGroup }

func (*SyntheticCreateChain) GetType() types.TxType { return types.TxTypeSyntheticCreateChain }

func (*SyntheticDepositCredits) GetType() types.TxType { return types.TxTypeSyntheticDepositCredits }

func (v *AddCredits) BinarySize() int {
	var n int

	n += uvarintBinarySize(uint64(types.TxTypeAddCredits))

	n += stringBinarySize(v.Recipient)

	n += uvarintBinarySize(v.Amount)

	return n
}

func (v *AnonTokenAccount) BinarySize() int {
	var n int

	// Enforce sanity
	v.Type = types.ChainTypeAnonTokenAccount

	n += v.ChainHeader.GetHeaderSize()

	n += stringBinarySize(v.TokenUrl)

	n += bigintBinarySize(&v.Balance)

	n += uvarintBinarySize(v.TxCount)

	n += uvarintBinarySize(v.Nonce)

	n += bigintBinarySize(&v.CreditBalance)

	return n
}

func (v *AssignSigSpecGroup) BinarySize() int {
	var n int

	n += uvarintBinarySize(uint64(types.TxTypeAssignSigSpecGroup))

	n += stringBinarySize(v.Url)

	return n
}

func (v *ChainParams) BinarySize() int {
	var n int

	n += stringBinarySize(v.Url)

	n += bytesBinarySize(v.Data)

	return n
}

func (v *CreateSigSpec) BinarySize() int {
	var n int

	n += uvarintBinarySize(uint64(types.TxTypeCreateSigSpec))

	n += stringBinarySize(v.Url)

	n += uvarintBinarySize(uint64(len(v.Keys)))

	for _, v := range v.Keys {
		n += v.BinarySize()

	}

	return n
}

func (v *CreateSigSpecGroup) BinarySize() int {
	var n int

	n += uvarintBinarySize(uint64(types.TxTypeCreateSigSpecGroup))

	n += stringBinarySize(v.Url)

	n += chainSetBinarySize(v.SigSpecs)

	return n
}

func (v *KeySpec) BinarySize() int {
	var n int

	n += v.KeyAlgorithm.BinarySize()

	n += v.HashAlgorithm.BinarySize()

	n += bytesBinarySize(v.PublicKey)

	n += uvarintBinarySize(v.Nonce)

	return n
}

func (v *KeySpecParams) BinarySize() int {
	var n int

	n += v.KeyAlgorithm.BinarySize()

	n += v.HashAlgorithm.BinarySize()

	n += bytesBinarySize(v.PublicKey)

	return n
}

func (v *SigSpec) BinarySize() int {
	var n int

	// Enforce sanity
	v.Type = types.ChainTypeSigSpec

	n += v.ChainHeader.GetHeaderSize()

	n += bigintBinarySize(&v.CreditBalance)

	n += uvarintBinarySize(uint64(len(v.Keys)))

	for _, v := range v.Keys {
		n += v.BinarySize()

	}

	return n
}

func (v *SigSpecGroup) BinarySize() int {
	var n int

	// Enforce sanity
	v.Type = types.ChainTypeSigSpecGroup

	n += v.ChainHeader.GetHeaderSize()

	n += chainSetBinarySize(v.SigSpecs)

	return n
}

func (v *SyntheticCreateChain) BinarySize() int {
	var n int

	n += uvarintBinarySize(uint64(types.TxTypeSyntheticCreateChain))

	n += chainBinarySize(&v.Cause)

	n += uvarintBinarySize(uint64(len(v.Chains)))

	for _, v := range v.Chains {
		n += bytesBinarySize(v)

	}

	return n
}

func (v *SyntheticDepositCredits) BinarySize() int {
	var n int

	n += uvarintBinarySize(uint64(types.TxTypeSyntheticDepositCredits))

	n += chainBinarySize(&v.Cause)

	n += uvarintBinarySize(v.Amount)

	return n
}

func (v *TxResult) BinarySize() int {
	var n int

	n += uvarintBinarySize(uint64(len(v.SyntheticTxs)))

	for _, v := range v.SyntheticTxs {
		n += v.BinarySize()

	}

	return n
}

func (v *TxSynthRef) BinarySize() int {
	var n int

	n += uvarintBinarySize(v.Type)

	n += chainBinarySize(&v.Hash)

	n += stringBinarySize(v.Url)

	n += chainBinarySize(&v.TxRef)

	return n
}

func (v *AddCredits) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(uvarintMarshalBinary(uint64(types.TxTypeAddCredits)))

	buffer.Write(stringMarshalBinary(v.Recipient))

	buffer.Write(uvarintMarshalBinary(v.Amount))

	return buffer.Bytes(), nil
}

func (v *AnonTokenAccount) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	// Enforce sanity
	v.Type = types.ChainTypeAnonTokenAccount

	if b, err := v.ChainHeader.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("error encoding header: %w", err)
	} else {
		buffer.Write(b)
	}
	buffer.Write(stringMarshalBinary(v.TokenUrl))

	buffer.Write(bigintMarshalBinary(&v.Balance))

	buffer.Write(uvarintMarshalBinary(v.TxCount))

	buffer.Write(uvarintMarshalBinary(v.Nonce))

	buffer.Write(bigintMarshalBinary(&v.CreditBalance))

	return buffer.Bytes(), nil
}

func (v *AssignSigSpecGroup) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(uvarintMarshalBinary(uint64(types.TxTypeAssignSigSpecGroup)))

	buffer.Write(stringMarshalBinary(v.Url))

	return buffer.Bytes(), nil
}

func (v *ChainParams) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(stringMarshalBinary(v.Url))

	buffer.Write(bytesMarshalBinary(v.Data))

	return buffer.Bytes(), nil
}

func (v *CreateSigSpec) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(uvarintMarshalBinary(uint64(types.TxTypeCreateSigSpec)))

	buffer.Write(stringMarshalBinary(v.Url))

	buffer.Write(uvarintMarshalBinary(uint64(len(v.Keys))))
	for i, v := range v.Keys {
		_ = i
		if b, err := v.MarshalBinary(); err != nil {
			return nil, fmt.Errorf("error encoding Keys[%d]: %w", i, err)
		} else {
			buffer.Write(b)
		}

	}

	return buffer.Bytes(), nil
}

func (v *CreateSigSpecGroup) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(uvarintMarshalBinary(uint64(types.TxTypeCreateSigSpecGroup)))

	buffer.Write(stringMarshalBinary(v.Url))

	buffer.Write(chainSetMarshalBinary(v.SigSpecs))

	return buffer.Bytes(), nil
}

func (v *KeySpec) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	if b, err := v.KeyAlgorithm.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("error encoding KeyAlgorithm: %w", err)
	} else {
		buffer.Write(b)
	}

	if b, err := v.HashAlgorithm.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("error encoding HashAlgorithm: %w", err)
	} else {
		buffer.Write(b)
	}

	buffer.Write(bytesMarshalBinary(v.PublicKey))

	buffer.Write(uvarintMarshalBinary(v.Nonce))

	return buffer.Bytes(), nil
}

func (v *KeySpecParams) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	if b, err := v.KeyAlgorithm.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("error encoding KeyAlgorithm: %w", err)
	} else {
		buffer.Write(b)
	}

	if b, err := v.HashAlgorithm.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("error encoding HashAlgorithm: %w", err)
	} else {
		buffer.Write(b)
	}

	buffer.Write(bytesMarshalBinary(v.PublicKey))

	return buffer.Bytes(), nil
}

func (v *SigSpec) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	// Enforce sanity
	v.Type = types.ChainTypeSigSpec

	if b, err := v.ChainHeader.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("error encoding header: %w", err)
	} else {
		buffer.Write(b)
	}
	buffer.Write(bigintMarshalBinary(&v.CreditBalance))

	buffer.Write(uvarintMarshalBinary(uint64(len(v.Keys))))
	for i, v := range v.Keys {
		_ = i
		if b, err := v.MarshalBinary(); err != nil {
			return nil, fmt.Errorf("error encoding Keys[%d]: %w", i, err)
		} else {
			buffer.Write(b)
		}

	}

	return buffer.Bytes(), nil
}

func (v *SigSpecGroup) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	// Enforce sanity
	v.Type = types.ChainTypeSigSpecGroup

	if b, err := v.ChainHeader.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("error encoding header: %w", err)
	} else {
		buffer.Write(b)
	}
	buffer.Write(chainSetMarshalBinary(v.SigSpecs))

	return buffer.Bytes(), nil
}

func (v *SyntheticCreateChain) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(uvarintMarshalBinary(uint64(types.TxTypeSyntheticCreateChain)))

	buffer.Write(chainMarshalBinary(&v.Cause))

	buffer.Write(uvarintMarshalBinary(uint64(len(v.Chains))))
	for i, v := range v.Chains {
		_ = i
		buffer.Write(bytesMarshalBinary(v))

	}

	return buffer.Bytes(), nil
}

func (v *SyntheticDepositCredits) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(uvarintMarshalBinary(uint64(types.TxTypeSyntheticDepositCredits)))

	buffer.Write(chainMarshalBinary(&v.Cause))

	buffer.Write(uvarintMarshalBinary(v.Amount))

	return buffer.Bytes(), nil
}

func (v *TxResult) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(uvarintMarshalBinary(uint64(len(v.SyntheticTxs))))
	for i, v := range v.SyntheticTxs {
		_ = i
		if b, err := v.MarshalBinary(); err != nil {
			return nil, fmt.Errorf("error encoding SyntheticTxs[%d]: %w", i, err)
		} else {
			buffer.Write(b)
		}

	}

	return buffer.Bytes(), nil
}

func (v *TxSynthRef) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(uvarintMarshalBinary(v.Type))

	buffer.Write(chainMarshalBinary(&v.Hash))

	buffer.Write(stringMarshalBinary(v.Url))

	buffer.Write(chainMarshalBinary(&v.TxRef))

	return buffer.Bytes(), nil
}

func (v *AddCredits) UnmarshalBinary(data []byte) error {
	typ := types.TxTypeAddCredits
	if v, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TX type: %w", err)
	} else if v != uint64(typ) {
		return fmt.Errorf("invalid TX type: want %v, got %v", typ, types.TxType(v))
	}
	data = data[uvarintBinarySize(uint64(typ)):]

	if x, err := stringUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Recipient: %w", err)
	} else {
		v.Recipient = x
	}
	data = data[stringBinarySize(v.Recipient):]

	if x, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Amount: %w", err)
	} else {
		v.Amount = x
	}
	data = data[uvarintBinarySize(v.Amount):]

	return nil
}

func (v *AnonTokenAccount) UnmarshalBinary(data []byte) error {
	typ := types.ChainTypeAnonTokenAccount
	if err := v.ChainHeader.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding header: %w", err)
	} else if v.Type != typ {
		return fmt.Errorf("invalid chain type: want %v, got %v", typ, v.Type)
	}
	data = data[v.GetHeaderSize():]

	if x, err := stringUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TokenUrl: %w", err)
	} else {
		v.TokenUrl = x
	}
	data = data[stringBinarySize(v.TokenUrl):]

	if x, err := bigintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Balance: %w", err)
	} else {
		v.Balance.Set(x)
	}
	data = data[bigintBinarySize(&v.Balance):]

	if x, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TxCount: %w", err)
	} else {
		v.TxCount = x
	}
	data = data[uvarintBinarySize(v.TxCount):]

	if x, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Nonce: %w", err)
	} else {
		v.Nonce = x
	}
	data = data[uvarintBinarySize(v.Nonce):]

	if x, err := bigintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding CreditBalance: %w", err)
	} else {
		v.CreditBalance.Set(x)
	}
	data = data[bigintBinarySize(&v.CreditBalance):]

	return nil
}

func (v *AssignSigSpecGroup) UnmarshalBinary(data []byte) error {
	typ := types.TxTypeAssignSigSpecGroup
	if v, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TX type: %w", err)
	} else if v != uint64(typ) {
		return fmt.Errorf("invalid TX type: want %v, got %v", typ, types.TxType(v))
	}
	data = data[uvarintBinarySize(uint64(typ)):]

	if x, err := stringUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Url: %w", err)
	} else {
		v.Url = x
	}
	data = data[stringBinarySize(v.Url):]

	return nil
}

func (v *ChainParams) UnmarshalBinary(data []byte) error {
	if x, err := stringUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Url: %w", err)
	} else {
		v.Url = x
	}
	data = data[stringBinarySize(v.Url):]

	if x, err := bytesUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Data: %w", err)
	} else {
		v.Data = x
	}
	data = data[bytesBinarySize(v.Data):]

	return nil
}

func (v *CreateSigSpec) UnmarshalBinary(data []byte) error {
	typ := types.TxTypeCreateSigSpec
	if v, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TX type: %w", err)
	} else if v != uint64(typ) {
		return fmt.Errorf("invalid TX type: want %v, got %v", typ, types.TxType(v))
	}
	data = data[uvarintBinarySize(uint64(typ)):]

	if x, err := stringUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Url: %w", err)
	} else {
		v.Url = x
	}
	data = data[stringBinarySize(v.Url):]

	var lenKeys uint64
	if x, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Keys: %w", err)
	} else {
		lenKeys = x
	}
	data = data[uvarintBinarySize(lenKeys):]

	v.Keys = make([]*KeySpecParams, lenKeys)
	for i := range v.Keys {
		x := new(KeySpecParams)
		if err := x.UnmarshalBinary(data); err != nil {
			return fmt.Errorf("error decoding Keys[%d]: %w", i, err)
		}
		data = data[x.BinarySize():]

		v.Keys[i] = x
	}

	return nil
}

func (v *CreateSigSpecGroup) UnmarshalBinary(data []byte) error {
	typ := types.TxTypeCreateSigSpecGroup
	if v, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TX type: %w", err)
	} else if v != uint64(typ) {
		return fmt.Errorf("invalid TX type: want %v, got %v", typ, types.TxType(v))
	}
	data = data[uvarintBinarySize(uint64(typ)):]

	if x, err := stringUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Url: %w", err)
	} else {
		v.Url = x
	}
	data = data[stringBinarySize(v.Url):]

	if x, err := chainSetUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding SigSpecs: %w", err)
	} else {
		v.SigSpecs = x
	}
	data = data[chainSetBinarySize(v.SigSpecs):]

	return nil
}

func (v *KeySpec) UnmarshalBinary(data []byte) error {
	if err := v.KeyAlgorithm.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding KeyAlgorithm: %w", err)
	}
	data = data[v.KeyAlgorithm.BinarySize():]

	if err := v.HashAlgorithm.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding HashAlgorithm: %w", err)
	}
	data = data[v.HashAlgorithm.BinarySize():]

	if x, err := bytesUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding PublicKey: %w", err)
	} else {
		v.PublicKey = x
	}
	data = data[bytesBinarySize(v.PublicKey):]

	if x, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Nonce: %w", err)
	} else {
		v.Nonce = x
	}
	data = data[uvarintBinarySize(v.Nonce):]

	return nil
}

func (v *KeySpecParams) UnmarshalBinary(data []byte) error {
	if err := v.KeyAlgorithm.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding KeyAlgorithm: %w", err)
	}
	data = data[v.KeyAlgorithm.BinarySize():]

	if err := v.HashAlgorithm.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding HashAlgorithm: %w", err)
	}
	data = data[v.HashAlgorithm.BinarySize():]

	if x, err := bytesUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding PublicKey: %w", err)
	} else {
		v.PublicKey = x
	}
	data = data[bytesBinarySize(v.PublicKey):]

	return nil
}

func (v *SigSpec) UnmarshalBinary(data []byte) error {
	typ := types.ChainTypeSigSpec
	if err := v.ChainHeader.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding header: %w", err)
	} else if v.Type != typ {
		return fmt.Errorf("invalid chain type: want %v, got %v", typ, v.Type)
	}
	data = data[v.GetHeaderSize():]

	if x, err := bigintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding CreditBalance: %w", err)
	} else {
		v.CreditBalance.Set(x)
	}
	data = data[bigintBinarySize(&v.CreditBalance):]

	var lenKeys uint64
	if x, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Keys: %w", err)
	} else {
		lenKeys = x
	}
	data = data[uvarintBinarySize(lenKeys):]

	v.Keys = make([]*KeySpec, lenKeys)
	for i := range v.Keys {
		x := new(KeySpec)
		if err := x.UnmarshalBinary(data); err != nil {
			return fmt.Errorf("error decoding Keys[%d]: %w", i, err)
		}
		data = data[x.BinarySize():]

		v.Keys[i] = x
	}

	return nil
}

func (v *SigSpecGroup) UnmarshalBinary(data []byte) error {
	typ := types.ChainTypeSigSpecGroup
	if err := v.ChainHeader.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding header: %w", err)
	} else if v.Type != typ {
		return fmt.Errorf("invalid chain type: want %v, got %v", typ, v.Type)
	}
	data = data[v.GetHeaderSize():]

	if x, err := chainSetUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding SigSpecs: %w", err)
	} else {
		v.SigSpecs = x
	}
	data = data[chainSetBinarySize(v.SigSpecs):]

	return nil
}

func (v *SyntheticCreateChain) UnmarshalBinary(data []byte) error {
	typ := types.TxTypeSyntheticCreateChain
	if v, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TX type: %w", err)
	} else if v != uint64(typ) {
		return fmt.Errorf("invalid TX type: want %v, got %v", typ, types.TxType(v))
	}
	data = data[uvarintBinarySize(uint64(typ)):]

	if x, err := chainUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Cause: %w", err)
	} else {
		v.Cause = x
	}
	data = data[chainBinarySize(&v.Cause):]

	var lenChains uint64
	if x, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Chains: %w", err)
	} else {
		lenChains = x
	}
	data = data[uvarintBinarySize(lenChains):]

	v.Chains = make([][]byte, lenChains)
	for i := range v.Chains {
		if x, err := bytesUnmarshalBinary(data); err != nil {
			return fmt.Errorf("error decoding Chains[%d]: %w", i, err)
		} else {
			v.Chains[i] = x
		}
		data = data[bytesBinarySize(v.Chains[i]):]

	}

	return nil
}

func (v *SyntheticDepositCredits) UnmarshalBinary(data []byte) error {
	typ := types.TxTypeSyntheticDepositCredits
	if v, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TX type: %w", err)
	} else if v != uint64(typ) {
		return fmt.Errorf("invalid TX type: want %v, got %v", typ, types.TxType(v))
	}
	data = data[uvarintBinarySize(uint64(typ)):]

	if x, err := chainUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Cause: %w", err)
	} else {
		v.Cause = x
	}
	data = data[chainBinarySize(&v.Cause):]

	if x, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Amount: %w", err)
	} else {
		v.Amount = x
	}
	data = data[uvarintBinarySize(v.Amount):]

	return nil
}

func (v *TxResult) UnmarshalBinary(data []byte) error {
	var lenSyntheticTxs uint64
	if x, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding SyntheticTxs: %w", err)
	} else {
		lenSyntheticTxs = x
	}
	data = data[uvarintBinarySize(lenSyntheticTxs):]

	v.SyntheticTxs = make([]*TxSynthRef, lenSyntheticTxs)
	for i := range v.SyntheticTxs {
		x := new(TxSynthRef)
		if err := x.UnmarshalBinary(data); err != nil {
			return fmt.Errorf("error decoding SyntheticTxs[%d]: %w", i, err)
		}
		data = data[x.BinarySize():]

		v.SyntheticTxs[i] = x
	}

	return nil
}

func (v *TxSynthRef) UnmarshalBinary(data []byte) error {
	if x, err := uvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Type: %w", err)
	} else {
		v.Type = x
	}
	data = data[uvarintBinarySize(v.Type):]

	if x, err := chainUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Hash: %w", err)
	} else {
		v.Hash = x
	}
	data = data[chainBinarySize(&v.Hash):]

	if x, err := stringUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Url: %w", err)
	} else {
		v.Url = x
	}
	data = data[stringBinarySize(v.Url):]

	if x, err := chainUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TxRef: %w", err)
	} else {
		v.TxRef = x
	}
	data = data[chainBinarySize(&v.TxRef):]

	return nil
}
