package state

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

type IdentityState struct {
	StateEntry
	keytype byte
	keydata []byte //this will eventually be the key groups
	adi     string
}

func NewIdentityState(adi string) *IdentityState {
	r := &IdentityState{}
	r.adi = adi
	return r
}

func (is *IdentityState) SetKeyData(keytype byte, data []byte) error {
	if len(data) > cap(is.keydata) {
		is.keydata = make([]byte, len(data))
	}
	is.keytype = keytype
	copy(is.keydata, data)

	return nil
}
func (is *IdentityState) GetKeyData() (byte, []byte) {
	return is.keytype, is.keydata
}

func (is *IdentityState) GetAdi() string {
	return is.adi
}

func (is *IdentityState) GetIdentityChainId() []byte {
	h := sha256.Sum256([]byte(is.adi)) //validator.BuildChainIdFromAdi(&is.adi)
	return h[:]
}

func (is *IdentityState) MarshalBinary() ([]byte, error) {

	data := make([]byte, 1+8+len(is.adi)+1+len(is.keydata))
	//length of adi
	data[0] = is.keytype

	i := 1
	//store the keydata size
	i += binary.PutVarint(data[i:], int64(len(is.keydata)))
	//store the key data
	i += copy(data[i:], is.keydata[:])

	//store the adi len
	ladi := byte(len(is.adi))
	data[i] = ladi
	i++
	//store the adi
	i += copy(data[i:], []byte(is.adi))

	data = data[:i]

	return data, nil
}

func (is *IdentityState) UnmarshalBinary(data []byte) error {
	dlen := len(data)
	if dlen == 0 {
		return fmt.Errorf("Cannot unmarshal Identity State, insuffient data")
	}
	i := 0
	is.keytype = data[i]
	i++
	if dlen <= i {
		return fmt.Errorf("Cannot unmarshal Identity State after key type, insuffient data")
	}
	v, l := binary.Varint(data[i:])
	if l <= 0 {
		return fmt.Errorf("Cannot unmarshal Identity State after key data len, insuffient data")
	}
	i += l

	if dlen < i {
		return fmt.Errorf("Cannot unmarshal Identity State before copy key data, insuffient data")
	}
	is.keydata = make([]byte, v)
	i += copy(is.keydata, data[i:i+int(v)])

	if dlen < i {
		return fmt.Errorf("Cannot unmarshal Identity State before adi len, insuffient data")
	}

	l = int(data[i])
	i++

	if dlen < i+l {
		return fmt.Errorf("Cannot unmarshal Identity State before copy adi, insuffient data")
	}

	is.adi = string(data[i : i+l])

	return nil
}

//
//func (app *IdentityState) MarshalEntry() (*Entry, error) {
//	return nil, nil
//}
//func (app *IdentityState) UnmarshalEntry(entry *Entry) error {
//	return nil
//}
