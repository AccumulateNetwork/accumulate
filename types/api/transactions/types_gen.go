package transactions

// GENERATED BY go run ./tools/cmd/genmarshal. DO NOT EDIT.

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/encoding"
	"github.com/AccumulateNetwork/accumulate/internal/url"
)

type Envelope struct {
	Signatures  []*ED25519Sig `json:"signatures,omitempty" form:"signatures" query:"signatures" validate:"required"`
	TxHash      []byte        `json:"txHash,omitempty" form:"txHash" query:"txHash" validate:"required"`
	Transaction *Transaction  `json:"transaction,omitempty" form:"transaction" query:"transaction" validate:"required"`
	hash        []byte
}

type Header struct {
	Origin        *url.URL `json:"origin,omitempty" form:"origin" query:"origin" validate:"required"`
	KeyPageHeight uint64   `json:"keyPageHeight,omitempty" form:"keyPageHeight" query:"keyPageHeight" validate:"required"`
	KeyPageIndex  uint64   `json:"keyPageIndex,omitempty" form:"keyPageIndex" query:"keyPageIndex" validate:"required"`
	Nonce         uint64   `json:"nonce,omitempty" form:"nonce" query:"nonce" validate:"required"`
}

type Transaction struct {
	Header
	Body []byte `json:"body,omitempty" form:"body" query:"body" validate:"required"`
	hash []byte
}

func (v *Envelope) Equal(u *Envelope) bool {
	if !(len(v.Signatures) == len(u.Signatures)) {
		return false
	}

	for i := range v.Signatures {
		v, u := v.Signatures[i], u.Signatures[i]
		if !(v.Equal(u)) {
			return false
		}

	}

	if !(bytes.Equal(v.TxHash, u.TxHash)) {
		return false
	}

	if !(v.Transaction.Equal(u.Transaction)) {
		return false
	}

	return true
}

func (v *Header) Equal(u *Header) bool {
	if !(v.Origin.Equal(u.Origin)) {
		return false
	}

	if !(v.KeyPageHeight == u.KeyPageHeight) {
		return false
	}

	if !(v.KeyPageIndex == u.KeyPageIndex) {
		return false
	}

	if !(v.Nonce == u.Nonce) {
		return false
	}

	return true
}

func (v *Transaction) Equal(u *Transaction) bool {
	if !(bytes.Equal(v.Body, u.Body)) {
		return false
	}

	return true
}

func (v *Envelope) BinarySize() int {
	var n int

	n += encoding.UvarintBinarySize(uint64(len(v.Signatures)))

	for _, v := range v.Signatures {
		n += v.BinarySize()

	}

	n += encoding.BytesBinarySize(v.TxHash)

	n += v.Transaction.BinarySize()

	return n
}

func (v *Header) BinarySize() int {
	var n int

	n += v.Origin.BinarySize()

	n += encoding.UvarintBinarySize(v.KeyPageHeight)

	n += encoding.UvarintBinarySize(v.KeyPageIndex)

	n += encoding.UvarintBinarySize(v.Nonce)

	return n
}

func (v *Transaction) BinarySize() int {
	var n int

	n += v.Header.BinarySize()

	n += encoding.BytesBinarySize(v.Body)

	return n
}

func (v *Envelope) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(encoding.UvarintMarshalBinary(uint64(len(v.Signatures))))
	for i, v := range v.Signatures {
		_ = i
		if b, err := v.MarshalBinary(); err != nil {
			return nil, fmt.Errorf("error encoding Signatures[%d]: %w", i, err)
		} else {
			buffer.Write(b)
		}

	}

	buffer.Write(encoding.BytesMarshalBinary(v.TxHash))

	if b, err := v.Transaction.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("error encoding Transaction: %w", err)
	} else {
		buffer.Write(b)
	}

	return buffer.Bytes(), nil
}

func (v *Header) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	if b, err := v.Origin.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("error encoding Origin: %w", err)
	} else {
		buffer.Write(b)
	}

	buffer.Write(encoding.UvarintMarshalBinary(v.KeyPageHeight))

	buffer.Write(encoding.UvarintMarshalBinary(v.KeyPageIndex))

	buffer.Write(encoding.UvarintMarshalBinary(v.Nonce))

	return buffer.Bytes(), nil
}

func (v *Transaction) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	if b, err := v.Header.MarshalBinary(); err != nil {
		return nil, err
	} else {
		buffer.Write(b)
	}

	buffer.Write(encoding.BytesMarshalBinary(v.Body))

	return buffer.Bytes(), nil
}

func (v *Envelope) UnmarshalBinary(data []byte) error {
	var lenSignatures uint64
	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Signatures: %w", err)
	} else {
		lenSignatures = x
	}
	data = data[encoding.UvarintBinarySize(lenSignatures):]

	v.Signatures = make([]*ED25519Sig, lenSignatures)
	for i := range v.Signatures {
		var x *ED25519Sig
		x = new(ED25519Sig)
		if err := x.UnmarshalBinary(data); err != nil {
			return fmt.Errorf("error decoding Signatures[%d]: %w", i, err)
		}
		data = data[x.BinarySize():]

		v.Signatures[i] = x
	}

	if x, err := encoding.BytesUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TxHash: %w", err)
	} else {
		v.TxHash = x
	}
	data = data[encoding.BytesBinarySize(v.TxHash):]

	v.Transaction = new(Transaction)
	if err := v.Transaction.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Transaction: %w", err)
	}
	data = data[v.Transaction.BinarySize():]

	return nil
}

func (v *Header) UnmarshalBinary(data []byte) error {
	v.Origin = new(url.URL)
	if err := v.Origin.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Origin: %w", err)
	}
	data = data[v.Origin.BinarySize():]

	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding KeyPageHeight: %w", err)
	} else {
		v.KeyPageHeight = x
	}
	data = data[encoding.UvarintBinarySize(v.KeyPageHeight):]

	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding KeyPageIndex: %w", err)
	} else {
		v.KeyPageIndex = x
	}
	data = data[encoding.UvarintBinarySize(v.KeyPageIndex):]

	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Nonce: %w", err)
	} else {
		v.Nonce = x
	}
	data = data[encoding.UvarintBinarySize(v.Nonce):]

	return nil
}

func (v *Transaction) UnmarshalBinary(data []byte) error {
	if err := v.Header.UnmarshalBinary(data); err != nil {
		return err
	}
	data = data[v.Header.BinarySize():]

	if x, err := encoding.BytesUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Body: %w", err)
	} else {
		v.Body = x
	}
	data = data[encoding.BytesBinarySize(v.Body):]

	return nil
}

func (v *Envelope) MarshalJSON() ([]byte, error) {
	u := struct {
		Signatures  []*ED25519Sig `json:"signatures,omitempty"`
		TxHash      *string       `json:"txHash,omitempty"`
		Transaction *Transaction  `json:"transaction,omitempty"`
	}{}
	u.Signatures = v.Signatures
	u.TxHash = encoding.BytesToJSON(v.TxHash)
	u.Transaction = v.Transaction
	return json.Marshal(&u)
}

func (v *Transaction) MarshalJSON() ([]byte, error) {
	u := struct {
		Header
		Body *string `json:"body,omitempty"`
	}{}
	u.Header = v.Header
	u.Body = encoding.BytesToJSON(v.Body)
	return json.Marshal(&u)
}

func (v *Envelope) UnmarshalJSON(data []byte) error {
	u := struct {
		Signatures  []*ED25519Sig `json:"signatures,omitempty"`
		TxHash      *string       `json:"txHash,omitempty"`
		Transaction *Transaction  `json:"transaction,omitempty"`
	}{}
	u.Signatures = v.Signatures
	u.TxHash = encoding.BytesToJSON(v.TxHash)
	u.Transaction = v.Transaction
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Signatures = u.Signatures
	if x, err := encoding.BytesFromJSON(u.TxHash); err != nil {
		return fmt.Errorf("error decoding TxHash: %w", err)
	} else {
		v.TxHash = x
	}
	v.Transaction = u.Transaction
	return nil
}

func (v *Transaction) UnmarshalJSON(data []byte) error {
	u := struct {
		Header
		Body *string `json:"body,omitempty"`
	}{}
	u.Header = v.Header
	u.Body = encoding.BytesToJSON(v.Body)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Header = u.Header
	if x, err := encoding.BytesFromJSON(u.Body); err != nil {
		return fmt.Errorf("error decoding Body: %w", err)
	} else {
		v.Body = x
	}
	return nil
}
