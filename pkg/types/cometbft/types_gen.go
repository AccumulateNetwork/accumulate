// Code generated by gitlab.com/accumulatenetwork/core/schema. DO NOT EDIT.

package cometbft

import (
	encoding "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	protocol "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
	"gitlab.com/accumulatenetwork/core/schema/pkg/widget"
)

type Block struct {
	Height     int64
	HeaderHash []byte
	AppHash    []byte
}

var wBlock = widget.ForComposite(widget.Fields[Block]{
	{Name: "height", ID: 1, Widget: widget.ForInt(func(v *Block) *int64 { return &v.Height })},
	{Name: "headerHash", ID: 2, Widget: widget.ForBytes(func(v *Block) *[]byte { return &v.HeaderHash })},
	{Name: "appHash", ID: 3, Widget: widget.ForBytes(func(v *Block) *[]byte { return &v.AppHash })},
}, widget.Identity[*Block])

// Copy returns a copy of the Block.
func (v *Block) Copy() *Block {
	if v == nil {
		return nil
	}
	var u = new(Block)
	wBlock.CopyTo(u, v)
	return u
}

// EqualBlock returns true if V is equal to U.
func (v *Block) Equal(u *Block) bool {
	return wBlock.Equal(v, u)
}

// MarshalBinary marshals the Block to JSON.
func (v *Block) MarshalJSON() ([]byte, error) {
	return widget.MarshalJSON(v, wBlock)
}

// UnmarshalJSON unmarshals the Block from JSON.
func (v *Block) UnmarshalJSON(b []byte) error {
	return widget.UnmarshalJSON(v, wBlock, b)
}

// MarshalBinary marshals the Block to bytes using [binary].
func (v *Block) MarshalBinary() ([]byte, error) {
	return widget.MarshalBinary(v, wBlock)
}

// MarshalBinary marshals the Block to a [binary.Encoder].
func (v *Block) MarshalBinaryV2(enc *binary.Encoder) error {
	return wBlock.MarshalBinary(enc, v)
}

// UnmarshalBinary unmarshals the Block from bytes using [binary].
func (v *Block) UnmarshalBinary(b []byte) error {
	return widget.UnmarshalBinary(v, wBlock, b)
}

// UnmarshalBinary unmarshals the Block from a [binary.Decoder].
func (v *Block) UnmarshalBinaryV2(dec *binary.Decoder) error {
	return wBlock.UnmarshalBinary(dec, v)
}

type GenesisDoc struct {
	ChainID    string
	Params     *ConsensusParams
	Validators []*Validator
	Block      *Block
}

var wGenesisDoc = widget.ForComposite(widget.Fields[GenesisDoc]{
	{Name: "chainID", ID: 1, Widget: widget.ForString(func(v *GenesisDoc) *string { return &v.ChainID })},
	{Name: "params", ID: 2, Widget: widget.ForValue(func(v *GenesisDoc) **ConsensusParams { return &v.Params })},
	{Name: "validators", ID: 3, Widget: widget.ForArray(widget.ForCompositePtr(wValidator.Fields, widget.GetElem[[]*Validator]), func(v *GenesisDoc) *[]*Validator { return &v.Validators })},
	{Name: "block", ID: 4, Widget: widget.ForCompositePtr(wBlock.Fields, func(v *GenesisDoc) **Block { return &v.Block })},
}, widget.Identity[*GenesisDoc])

// Copy returns a copy of the GenesisDoc.
func (v *GenesisDoc) Copy() *GenesisDoc {
	if v == nil {
		return nil
	}
	var u = new(GenesisDoc)
	wGenesisDoc.CopyTo(u, v)
	return u
}

// EqualGenesisDoc returns true if V is equal to U.
func (v *GenesisDoc) Equal(u *GenesisDoc) bool {
	return wGenesisDoc.Equal(v, u)
}

// MarshalBinary marshals the GenesisDoc to JSON.
func (v *GenesisDoc) MarshalJSON() ([]byte, error) {
	return widget.MarshalJSON(v, wGenesisDoc)
}

// UnmarshalJSON unmarshals the GenesisDoc from JSON.
func (v *GenesisDoc) UnmarshalJSON(b []byte) error {
	return widget.UnmarshalJSON(v, wGenesisDoc, b)
}

// MarshalBinary marshals the GenesisDoc to bytes using [binary].
func (v *GenesisDoc) MarshalBinary() ([]byte, error) {
	return widget.MarshalBinary(v, wGenesisDoc)
}

// MarshalBinary marshals the GenesisDoc to a [binary.Encoder].
func (v *GenesisDoc) MarshalBinaryV2(enc *binary.Encoder) error {
	return wGenesisDoc.MarshalBinary(enc, v)
}

// UnmarshalBinary unmarshals the GenesisDoc from bytes using [binary].
func (v *GenesisDoc) UnmarshalBinary(b []byte) error {
	return widget.UnmarshalBinary(v, wGenesisDoc, b)
}

// UnmarshalBinary unmarshals the GenesisDoc from a [binary.Decoder].
func (v *GenesisDoc) UnmarshalBinaryV2(dec *binary.Decoder) error {
	return wGenesisDoc.UnmarshalBinary(dec, v)
}

type Validator struct {
	Address []byte
	Type    protocol.SignatureType
	PubKey  []byte
	Power   int64
	Name    string
}

var wValidator = widget.ForComposite(widget.Fields[Validator]{
	{Name: "address", ID: 1, Widget: widget.ForBytes(func(v *Validator) *[]byte { return &v.Address })},
	{Name: "type", ID: 2, Widget: widget.For(encoding.EnumWidgetFor, func(v *Validator) *protocol.SignatureType { return &v.Type })},
	{Name: "pubKey", ID: 3, Widget: widget.ForBytes(func(v *Validator) *[]byte { return &v.PubKey })},
	{Name: "power", ID: 4, Widget: widget.ForInt(func(v *Validator) *int64 { return &v.Power })},
	{Name: "name", ID: 5, Widget: widget.ForString(func(v *Validator) *string { return &v.Name })},
}, widget.Identity[*Validator])

// Copy returns a copy of the Validator.
func (v *Validator) Copy() *Validator {
	if v == nil {
		return nil
	}
	var u = new(Validator)
	wValidator.CopyTo(u, v)
	return u
}

// EqualValidator returns true if V is equal to U.
func (v *Validator) Equal(u *Validator) bool {
	return wValidator.Equal(v, u)
}

// MarshalBinary marshals the Validator to JSON.
func (v *Validator) MarshalJSON() ([]byte, error) {
	return widget.MarshalJSON(v, wValidator)
}

// UnmarshalJSON unmarshals the Validator from JSON.
func (v *Validator) UnmarshalJSON(b []byte) error {
	return widget.UnmarshalJSON(v, wValidator, b)
}

// MarshalBinary marshals the Validator to bytes using [binary].
func (v *Validator) MarshalBinary() ([]byte, error) {
	return widget.MarshalBinary(v, wValidator)
}

// MarshalBinary marshals the Validator to a [binary.Encoder].
func (v *Validator) MarshalBinaryV2(enc *binary.Encoder) error {
	return wValidator.MarshalBinary(enc, v)
}

// UnmarshalBinary unmarshals the Validator from bytes using [binary].
func (v *Validator) UnmarshalBinary(b []byte) error {
	return widget.UnmarshalBinary(v, wValidator, b)
}

// UnmarshalBinary unmarshals the Validator from a [binary.Decoder].
func (v *Validator) UnmarshalBinaryV2(dec *binary.Decoder) error {
	return wValidator.UnmarshalBinary(dec, v)
}
