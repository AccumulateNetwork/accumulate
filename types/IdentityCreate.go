package types

import "fmt"

//{ “identity-name” : “<new identity name>”,
//“identity-key-hash” : “<hex_pu”blic_key_hash>” }

type IdentityCreate struct {
	IdentityName    String  `json:"identity-name"`
	IdentityKeyHash Bytes32 `json:"identity-key-hash"`
}

func NewIdentityCreate(name string, keyhash *Bytes32) *IdentityCreate {
	ic := &IdentityCreate{}
	ic.SetName(name)
	if keyhash != nil {
		copy(ic.IdentityKeyHash[:], keyhash[:])
	}
	return ic
}

func (ic *IdentityCreate) SetName(name string) {
	ic.IdentityName = String(name)
}

func (ic *IdentityCreate) SetKeyHash(hash *Bytes32) {
	if hash != nil {
		copy(ic.IdentityKeyHash[:], hash[:])
	}
}

func (ic *IdentityCreate) MarshalBinary() ([]byte, error) {
	idn, err := ic.IdentityName.MarshalBinary()
	if err != nil {
		return nil, err
	}

	data := make([]byte, len(idn)+32)
	i := copy(data, idn)
	copy(data[i:], ic.IdentityKeyHash.Bytes())
	return data, nil
}

func (ic *IdentityCreate) UnmarshalBinary(data []byte) error {
	err := ic.IdentityName.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	l := ic.IdentityName.Size(nil)
	if len(data) < l+int(32) {
		return fmt.Errorf("Key hash length too short for identity create")
	}

	copy(ic.IdentityKeyHash[:], data[l:])

	return nil
}
