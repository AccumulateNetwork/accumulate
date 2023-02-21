// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"bytes"
	"io"
	"strconv"
	"strings"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// N_ACC is the multicodec name for the acc protocol.
const N_ACC = "acc"

// P_ACC is the multicodec code for the acc protocol.
const P_ACC = 0x300000

func init() {
	// Register the acc protocol
	err := multiaddr.AddProtocol(multiaddr.Protocol{
		Name:       N_ACC,
		Code:       P_ACC,
		VCode:      multiaddr.CodeToVarint(P_ACC),
		Size:       -1,
		Transcoder: transcoder{},
	})
	if err != nil {
		panic(err)
	}
}

type transcoder struct{}

// StringToBytes implements [multiaddr.Transcoder].
func (transcoder) StringToBytes(s string) ([]byte, error) {
	v, err := ParseServiceAddress(s)
	if err != nil {
		return nil, errors.BadRequest.Wrap(err)
	}
	b, err := v.MarshalBinary()
	if err != nil {
		return nil, errors.EncodingError.Wrap(err)
	}
	return b, nil
}

// BytesToString implements [multiaddr.Transcoder].
func (transcoder) BytesToString(b []byte) (string, error) {
	v := new(ServiceAddress)
	err := v.UnmarshalBinary(b)
	if err != nil {
		return "", errors.EncodingError.Wrap(err)
	}
	return v.String(), nil
}

// ValidateBytes implements [multiaddr.Transcoder].
func (transcoder) ValidateBytes(b []byte) error {
	v := new(ServiceAddress)
	err := v.UnmarshalBinary(b)
	return errors.EncodingError.Wrap(err)
}

// ParseServiceAddress parses a string as a [ServiceAddress]. See
// [ServiceAddress.String].
func ParseServiceAddress(s string) (*ServiceAddress, error) {
	parts := strings.Split(s, ":")
	if len(parts) > 2 {
		return nil, errors.BadRequest.With("too many parts")
	}

	// Parse as a known type or a number
	typ, ok := ServiceTypeByName(parts[0])
	if !ok {
		v, err := strconv.ParseUint(parts[0], 16, 64)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("invalid service type %q", parts[0])
		} else {
			typ = ServiceType(v)
		}
	}

	a := new(ServiceAddress)
	a.Type = typ
	if len(parts) > 1 {
		a.Partition = parts[1]
	}
	return a, nil
}

// String returns {type}:{partition}, or {type} if the partition is empty.
func (s *ServiceAddress) String() string {
	var str string
	var x ServiceType
	if x.SetEnumValue(s.Type.GetEnumValue()) {
		str = s.Type.String()
	} else {
		str = strconv.FormatUint(s.Type.GetEnumValue(), 16)
	}
	if s.Partition != "" {
		str += ":" + strings.ToLower(s.Partition)
	}
	return str
}

// Compare this address to another.
func (s *ServiceAddress) Compare(r *ServiceAddress) int {
	if s.Type != r.Type {
		return int(s.Type - r.Type)
	}

	// https://github.com/golang/go/issues/57314
	return strings.Compare(strings.ToLower(s.Partition), strings.ToLower(r.Partition))
}

// Equal returns true if the addresses are the same.
func (s *ServiceAddress) Equal(r *ServiceAddress) bool {
	return s.Compare(r) == 0
}

// Copy returns a copy of the address.
func (s *ServiceAddress) Copy() *ServiceAddress {
	return &ServiceAddress{Type: s.Type, Partition: s.Partition}
}

var fieldNames_ServiceAddress = []string{
	1: "Type",
	2: "Partition",
}

func (v *ServiceAddress) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Type == 0) {
		writer.WriteEnum(1, v.Type)
	}
	if !(len(v.Partition) == 0) {
		writer.WriteString(2, v.Partition)
	}

	_, _, err := writer.Reset(fieldNames_ServiceAddress)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *ServiceAddress) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *ServiceAddress) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadUint(1); ok {
		v.Type = ServiceType(x)
	}
	if x, ok := reader.ReadString(2); ok {
		v.Partition = x
	}

	seen, err := reader.Reset(fieldNames_ServiceAddress)
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
