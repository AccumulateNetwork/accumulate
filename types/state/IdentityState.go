package state

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"strings"
)

type KeyType byte

const (
	KeyType_unknown KeyType = iota
	KeyType_public
	KeyType_sha256
	KeyType_sha256d
	KeyType_group
	KeyType_chain
)

type identityState struct {
	StateEntry
	Type         string      `json:"type"`
	AdiChainPath string      `json:"adi-chain-path"`
	Keytype      KeyType     `json:"keytype"`
	Keydata      types.Bytes `json:"keydata"`
}

type IdentityState struct {
	StateEntry
	identityState
}

//this will eventually be the key groups and potentially just a multimap of types to chain paths controlled by the identity
func NewIdentityState(adi string) *IdentityState {
	r := &IdentityState{}
	r.AdiChainPath = adi
	r.Type = "AIM-1"
	return r
}

func (is *IdentityState) GetType() string {
	return is.Type
}

func (is *IdentityState) GetAdiChainPath() string {
	return is.AdiChainPath
}

//currently key data is loosly defined based upon keytype.  This will
//be replaced once a formal spec for key groups is established.
//we will also be storing references to chains managed by the identity.
//a chain will most likely be just the chain paths mapped to chain types
func (is *IdentityState) SetKeyData(keytype KeyType, data []byte) error {
	if len(data) > cap(is.Keydata) {
		is.Keydata = make([]byte, len(data))
	}
	is.Keytype = keytype
	copy(is.Keydata, data)

	return nil
}

//Currently this will just return the key information
//in the future the identity will hold links to a bunch of subchains
//managed by the identities.  one of them will be of key groups.
func (is *IdentityState) GetKeyData() (KeyType, types.Bytes) {
	return is.Keytype, is.Keydata
}

func (is *IdentityState) GetIdentityChainId() types.Bytes {
	h := types.GetIdentityChainFromIdentity(is.AdiChainPath)
	if h == nil {
		return types.Bytes{}
	}
	return h[:]
}

func (is *IdentityState) MarshalBinary() ([]byte, error) {

	data := make([]byte, 1+8+len(is.AdiChainPath)+1+len(is.Keydata))
	//length of adi
	i := 0
	i += copy(data[i:], []byte(is.Type)[:5])

	data[i] = byte(is.Keytype)

	i++
	//store the keydata size
	i += binary.PutVarint(data[i:], int64(len(is.Keydata)))
	//store the key data
	i += copy(data[i:], is.Keydata[:])

	//store the adi len
	ladi := byte(len(is.AdiChainPath))
	data[i] = ladi
	i++
	//store the adi
	i += copy(data[i:], []byte(is.AdiChainPath))

	data = data[:i]

	return data, nil
}

func (is *IdentityState) UnmarshalBinary(data []byte) error {
	dlen := len(data)
	if dlen == 0 {
		return fmt.Errorf("Cannot unmarshal Identity State, insuffient data")
	}
	i := 0
	if dlen < i+5 {
		return fmt.Errorf("Cannot unmarshal specification type")
	}
	is.Type = string(data[i:5])
	i += 5
	is.Keytype = KeyType(data[i])
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
	is.Keydata = make([]byte, v)
	i += copy(is.Keydata, data[i:i+int(v)])

	if dlen < i {
		return fmt.Errorf("Cannot unmarshal Identity State before adi len, insuffient data")
	}

	l = int(data[i])
	i++

	if dlen < i+l {
		return fmt.Errorf("Cannot unmarshal Identity State before copy adi, insuffient data")
	}

	is.AdiChainPath = string(data[i : i+l])

	return nil
}

func (is *IdentityState) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, is.identityState)
}

func (is *IdentityState) MarshalJSON() ([]byte, error) {
	return json.Marshal(is.identityState)
}

func (k *KeyType) UnmarshalJSON(b []byte) error {
	str := strings.Trim(string(b), `"`)

	switch {
	case str == "KeyType_public":
		*k = KeyType_public
	case str == "KeyType_sha256":
		*k = KeyType_sha256
	case str == "KeyType_sha256d":
		*k = KeyType_sha256d
	default:
		*k = KeyType_unknown
	}

	return nil
}
