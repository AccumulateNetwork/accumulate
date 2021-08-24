package api

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
)

// ADI structure holds the identity name in the URL.  The name can be stored as acc://<name> or simply <name>
// all chain paths following the ADI domain will be ignored
type ADI struct {
	URL           types.String  `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	PublicKeyHash types.Bytes32 `json:"publicKeyHash" form:"publicKeyHash" query:"publicKeyHash" validate:"required"` //",hexadecimal"`
}

func NewADI(name string, keyHash *types.Bytes32) *ADI {
	ic := &ADI{}
	ic.SetName(name)
	ic.SetKeyHash(keyHash)
	return ic
}

func (ic *ADI) SetName(name string) error {
	adi, _, err := types.ParseIdentityChainPath(name)

	if err != nil {
		return err
	}
	ic.URL = types.String(adi)
	return nil
}

func (ic *ADI) SetKeyHash(hash *types.Bytes32) {
	if hash != nil {
		copy(ic.PublicKeyHash[:], hash[:])
	}
}

func (ic *ADI) MarshalBinary() ([]byte, error) {
	idn, err := ic.URL.MarshalBinary()
	if err != nil {
		return nil, err
	}

	data := make([]byte, len(idn)+32)
	i := copy(data, idn)
	copy(data[i:], ic.PublicKeyHash.Bytes())
	return data, nil
}

func (ic *ADI) UnmarshalBinary(data []byte) error {
	err := ic.URL.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	l := ic.URL.Size(nil)
	if len(data) < l+32 {
		return fmt.Errorf("key hash length too short for identity create")
	}

	copy(ic.PublicKeyHash[:], data[l:])

	return nil
}
