package api

import (
	"strings"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

var enUsIgnoreCase = collate.New(language.AmericanEnglish, collate.IgnoreCase)

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

	typ, ok := ServiceTypeByName(parts[0])
	if !ok {
		return nil, errors.BadRequest.WithFormat("invalid service type %q", parts[0])
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
	str := s.Type.String()
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
	return enUsIgnoreCase.CompareString(s.Partition, r.Partition)
}

// Equal returns true if the addresses are the same.
func (s *ServiceAddress) Equal(r *ServiceAddress) bool {
	return s.Compare(r) == 0
}

// Copy returns a copy of the address.
func (s *ServiceAddress) Copy() *ServiceAddress {
	return &ServiceAddress{Type: s.Type, Partition: s.Partition}
}
