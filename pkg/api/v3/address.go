// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// N_ACC is the multicodec name for the acc protocol.
const N_ACC = "acc"

// P_ACC is the multicodec code for the acc protocol.
const P_ACC = 0x300000

// N_ACC_SVC is the multicodec name for the acc-svc protocol.
const N_ACC_SVC = "acc-svc"

// P_ACC_SVC is the multicodec code for the acc-svc protocol.
const P_ACC_SVC = 0x300001

// Address constructs a ServiceAddress for the service type.
func (s ServiceType) Address() *ServiceAddress {
	return &ServiceAddress{Type: s}
}

// AddressFor constructs a ServiceAddress for the service type and given
// argument.
func (s ServiceType) AddressFor(arg string) *ServiceAddress {
	return &ServiceAddress{Type: s, Argument: arg}
}

// ParseServiceAddress parses a string as a [ServiceAddress]. See
// [ServiceAddress.String].
func ParseServiceAddress(s string) (*ServiceAddress, error) {
	a := new(ServiceAddress)
	parts := strings.SplitN(s, ":", 2)
	if len(parts) > 1 {
		a.Argument = parts[1]
	}

	// Parse as a known type
	var ok bool
	a.Type, ok = ServiceTypeByName(parts[0])
	if ok {
		return a, nil
	}

	// Or as a hex number
	v, err := strconv.ParseUint(parts[0], 16, 64)
	if err == nil {
		a.Type = ServiceType(v)
		return a, nil
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
	if s.Argument != "" {
		str += ":" + strings.ToLower(s.Argument)
	}
	return str
}

// Compare this address to another.
func (s *ServiceAddress) Compare(r *ServiceAddress) int {
	// https://github.com/golang/go/issues/57314

	if s.Type != r.Type {
		return int(s.Type - r.Type)
	}

	return strings.Compare(strings.ToLower(s.Argument), strings.ToLower(r.Argument))
}

// Equal returns true if the addresses are the same.
func (s *ServiceAddress) Equal(r *ServiceAddress) bool {
	return s.Compare(r) == 0
}

// Copy returns a copy of the address.
func (s *ServiceAddress) Copy() *ServiceAddress {
	return &ServiceAddress{Type: s.Type, Argument: s.Argument}
}

// Multiaddr returns `/acc-svc/<type>[:<argument>]` as a multiaddr component.
func (s *ServiceAddress) Multiaddr() multiaddr.Multiaddr {
	c, err := multiaddr.NewComponent(N_ACC_SVC, s.String())
	if err != nil {
		// This only fails if the service isn't registered or parsing the string
		// fails. The service is registered by init() in this file, so that must
		// not fail. String and the parsing function are reciprocal, so that
		// must not fail. Thus if something fails it means the developers of
		// this code failed.
		panic(err)
	}
	return c
}

// MultiaddrFor returns `/acc/<network>/acc-svc/<type>[:<argument>]` as a
// multiaddr. MultiaddrFor returns an error if the network argument is not a
// valid UTF-8 string.
func (s *ServiceAddress) MultiaddrFor(network string) (multiaddr.Multiaddr, error) {
	c, err := multiaddr.NewComponent(N_ACC, network)
	if err != nil {
		return nil, err
	}
	return c.Encapsulate(s.Multiaddr()), nil
}

func init() {
	// Register the acc protocol
	err := multiaddr.AddProtocol(multiaddr.Protocol{
		Name:       N_ACC,
		Code:       P_ACC,
		VCode:      multiaddr.CodeToVarint(P_ACC),
		Size:       multiaddr.LengthPrefixedVarSize,
		Transcoder: stringTranscoder{},
	})
	if err != nil {
		panic(err)
	}

	// Register the acc-svc protocol
	err = multiaddr.AddProtocol(multiaddr.Protocol{
		Name:       N_ACC_SVC,
		Code:       P_ACC_SVC,
		VCode:      multiaddr.CodeToVarint(P_ACC_SVC),
		Size:       multiaddr.LengthPrefixedVarSize,
		Transcoder: serviceAddressTranscoder{},
	})
	if err != nil {
		panic(err)
	}
}

type stringTranscoder struct{}

// StringToBytes implements [multiaddr.Transcoder].
func (stringTranscoder) StringToBytes(s string) ([]byte, error) {
	if !utf8.ValidString(s) {
		return nil, errors.EncodingError.With("invalid UTF-8 string")
	}
	return []byte(s), nil
}

// BytesToString implements [multiaddr.Transcoder].
func (stringTranscoder) BytesToString(b []byte) (string, error) {
	if !utf8.Valid(b) {
		return "", errors.EncodingError.With("invalid UTF-8 string")
	}
	return string(b), nil
}

// ValidateBytes implements [multiaddr.Transcoder].
func (stringTranscoder) ValidateBytes(b []byte) error {
	if !utf8.Valid(b) {
		return errors.EncodingError.With("invalid UTF-8 string")
	}
	return nil
}

type serviceAddressTranscoder struct{}

// StringToBytes implements [multiaddr.Transcoder].
func (serviceAddressTranscoder) StringToBytes(s string) ([]byte, error) {
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
func (serviceAddressTranscoder) BytesToString(b []byte) (string, error) {
	v := new(ServiceAddress)
	err := v.UnmarshalBinary(b)
	if err != nil {
		return "", errors.EncodingError.Wrap(err)
	}
	return v.String(), nil
}

// ValidateBytes implements [multiaddr.Transcoder].
func (serviceAddressTranscoder) ValidateBytes(b []byte) error {
	v := new(ServiceAddress)
	err := v.UnmarshalBinary(b)
	return errors.EncodingError.Wrap(err)
}
