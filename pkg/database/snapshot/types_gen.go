// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Header struct {
	fieldsSet []bool
	// Version is the snapshot format version.
	Version uint64 `json:"version,omitempty" form:"version" query:"version" validate:"required"`
	// RootHash is the snapshot's root hash.
	RootHash [32]byte `json:"rootHash,omitempty" form:"rootHash" query:"rootHash" validate:"required"`
	// SystemLedger is the partition's system ledger.
	SystemLedger *protocol.SystemLedger `json:"systemLedger,omitempty" form:"systemLedger" query:"systemLedger" validate:"required"`
	extraData    []byte
}

type recordEntry struct {
	fieldsSet []bool
	Key       *record.Key `json:"key,omitempty" form:"key" query:"key" validate:"required"`
	Value     []byte      `json:"value,omitempty" form:"value" query:"value" validate:"required"`
	extraData []byte
}

type versionHeader struct {
	fieldsSet []bool
	// Version is the snapshot format version.
	Version   uint64 `json:"version,omitempty" form:"version" query:"version" validate:"required"`
	extraData []byte
}

func (v *Header) Copy() *Header {
	u := new(Header)

	u.Version = v.Version
	u.RootHash = v.RootHash
	if v.SystemLedger != nil {
		u.SystemLedger = (v.SystemLedger).Copy()
	}
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *Header) CopyAsInterface() interface{} { return v.Copy() }

func (v *recordEntry) Copy() *recordEntry {
	u := new(recordEntry)

	if v.Key != nil {
		u.Key = (v.Key).Copy()
	}
	u.Value = encoding.BytesCopy(v.Value)
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *recordEntry) CopyAsInterface() interface{} { return v.Copy() }

func (v *versionHeader) Copy() *versionHeader {
	u := new(versionHeader)

	u.Version = v.Version
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *versionHeader) CopyAsInterface() interface{} { return v.Copy() }

func (v *Header) Equal(u *Header) bool {
	if !(v.Version == u.Version) {
		return false
	}
	if !(v.RootHash == u.RootHash) {
		return false
	}
	switch {
	case v.SystemLedger == u.SystemLedger:
		// equal
	case v.SystemLedger == nil || u.SystemLedger == nil:
		return false
	case !((v.SystemLedger).Equal(u.SystemLedger)):
		return false
	}

	return true
}

func (v *recordEntry) Equal(u *recordEntry) bool {
	switch {
	case v.Key == u.Key:
		// equal
	case v.Key == nil || u.Key == nil:
		return false
	case !((v.Key).Equal(u.Key)):
		return false
	}
	if !(bytes.Equal(v.Value, u.Value)) {
		return false
	}

	return true
}

func (v *versionHeader) Equal(u *versionHeader) bool {
	if !(v.Version == u.Version) {
		return false
	}

	return true
}

var fieldNames_Header = []string{
	1: "Version",
	2: "RootHash",
	3: "SystemLedger",
}

func (v *Header) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Version == 0) {
		writer.WriteUint(1, v.Version)
	}
	if !(v.RootHash == ([32]byte{})) {
		writer.WriteHash(2, &v.RootHash)
	}
	if !(v.SystemLedger == nil) {
		writer.WriteValue(3, v.SystemLedger.MarshalBinary)
	}

	_, _, err := writer.Reset(fieldNames_Header)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *Header) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Version is missing")
	} else if v.Version == 0 {
		errs = append(errs, "field Version is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field RootHash is missing")
	} else if v.RootHash == ([32]byte{}) {
		errs = append(errs, "field RootHash is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field SystemLedger is missing")
	} else if v.SystemLedger == nil {
		errs = append(errs, "field SystemLedger is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_recordEntry = []string{
	1: "Key",
	2: "Value",
}

func (v *recordEntry) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Key == nil) {
		writer.WriteValue(1, v.Key.MarshalBinary)
	}
	if !(len(v.Value) == 0) {
		writer.WriteBytes(2, v.Value)
	}

	_, _, err := writer.Reset(fieldNames_recordEntry)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *recordEntry) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Key is missing")
	} else if v.Key == nil {
		errs = append(errs, "field Key is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Value is missing")
	} else if len(v.Value) == 0 {
		errs = append(errs, "field Value is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_versionHeader = []string{
	1: "Version",
}

func (v *versionHeader) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Version == 0) {
		writer.WriteUint(1, v.Version)
	}

	_, _, err := writer.Reset(fieldNames_versionHeader)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *versionHeader) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Version is missing")
	} else if v.Version == 0 {
		errs = append(errs, "field Version is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

func (v *Header) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Header) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadUint(1); ok {
		v.Version = x
	}
	if x, ok := reader.ReadHash(2); ok {
		v.RootHash = *x
	}
	if x := new(protocol.SystemLedger); reader.ReadValue(3, x.UnmarshalBinaryFrom) {
		v.SystemLedger = x
	}

	seen, err := reader.Reset(fieldNames_Header)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *recordEntry) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *recordEntry) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(record.Key); reader.ReadValue(1, x.UnmarshalBinaryFrom) {
		v.Key = x
	}
	if x, ok := reader.ReadBytes(2); ok {
		v.Value = x
	}

	seen, err := reader.Reset(fieldNames_recordEntry)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *versionHeader) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *versionHeader) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadUint(1); ok {
		v.Version = x
	}

	seen, err := reader.Reset(fieldNames_versionHeader)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *Header) MarshalJSON() ([]byte, error) {
	u := struct {
		Version      uint64                 `json:"version,omitempty"`
		RootHash     string                 `json:"rootHash,omitempty"`
		SystemLedger *protocol.SystemLedger `json:"systemLedger,omitempty"`
	}{}
	if !(v.Version == 0) {
		u.Version = v.Version
	}
	if !(v.RootHash == ([32]byte{})) {
		u.RootHash = encoding.ChainToJSON(v.RootHash)
	}
	if !(v.SystemLedger == nil) {
		u.SystemLedger = v.SystemLedger
	}
	return json.Marshal(&u)
}

func (v *recordEntry) MarshalJSON() ([]byte, error) {
	u := struct {
		Key   *record.Key `json:"key,omitempty"`
		Value *string     `json:"value,omitempty"`
	}{}
	if !(v.Key == nil) {
		u.Key = v.Key
	}
	if !(len(v.Value) == 0) {
		u.Value = encoding.BytesToJSON(v.Value)
	}
	return json.Marshal(&u)
}

func (v *Header) UnmarshalJSON(data []byte) error {
	u := struct {
		Version      uint64                 `json:"version,omitempty"`
		RootHash     string                 `json:"rootHash,omitempty"`
		SystemLedger *protocol.SystemLedger `json:"systemLedger,omitempty"`
	}{}
	u.Version = v.Version
	u.RootHash = encoding.ChainToJSON(v.RootHash)
	u.SystemLedger = v.SystemLedger
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Version = u.Version
	if x, err := encoding.ChainFromJSON(u.RootHash); err != nil {
		return fmt.Errorf("error decoding RootHash: %w", err)
	} else {
		v.RootHash = x
	}
	v.SystemLedger = u.SystemLedger
	return nil
}

func (v *recordEntry) UnmarshalJSON(data []byte) error {
	u := struct {
		Key   *record.Key `json:"key,omitempty"`
		Value *string     `json:"value,omitempty"`
	}{}
	u.Key = v.Key
	u.Value = encoding.BytesToJSON(v.Value)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Key = u.Key
	if x, err := encoding.BytesFromJSON(u.Value); err != nil {
		return fmt.Errorf("error decoding Value: %w", err)
	} else {
		v.Value = x
	}
	return nil
}
