package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"strings"
)

type KeyType byte

const (
	KeyTypeUnknown KeyType = iota
	KeyTypePublic
	KeyTypeSha256
	KeyTypeSha256d
	KeyTypeChain
	KeyTypeGroup
)

type identityState struct {
	Header
	Keytype KeyType     `json:"keytype"`
	Keydata types.Bytes `json:"keydata"`
}

type IdentityState struct {
	Entry
	identityState
}

// NewIdentityState this will eventually be the key groups and potentially just a multimap of types to chain paths controlled by the identity
func NewIdentityState(adi string) *IdentityState {
	r := &IdentityState{}
	r.AdiChainPath = types.String(adi)
	r.Type = "AIM-1"
	return r
}

func (is *IdentityState) GetAdiChainPath() string {
	return is.Header.GetAdiChainPath()
}

func (is *IdentityState) GetType() string {
	return is.Header.GetType()
}

func (is *IdentityState) VerifyKey(key []byte) bool {
	//check if key is a valid public key for identity
	if key[0] == is.Keydata[0] {
		if bytes.Compare(key, is.Keydata) == 0 {
			return true
		}
	}

	//check if key is a valid sha256(key) for identity
	kh := sha256.Sum256(key)
	if kh[0] == is.Keydata[0] {
		if bytes.Compare(key, kh[:]) == 0 {
			return true
		}
	}

	//check if key is a valid sha256d(key) for identity
	kh = sha256.Sum256(kh[:])
	if kh[0] == is.Keydata[0] {
		if bytes.Compare(key, kh[:]) == 0 {
			return true
		}
	}

	return false
}

//SetKeyData currently key data is loosly defined based upon keytype.  This will
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

//GetKeyData Currently this will just return the key information
//in the future the identity will hold links to a bunch of subchains
//managed by the identities.  one of them will be of key groups.
func (is *IdentityState) GetKeyData() (KeyType, types.Bytes) {
	return is.Keytype, is.Keydata
}

func (is *IdentityState) GetIdentityChainId() types.Bytes {
	h := types.GetIdentityChainFromIdentity(is.GetAdiChainPath())
	if h == nil {
		return types.Bytes{}
	}
	return h[:]
}

func (is *IdentityState) MarshalBinary() ([]byte, error) {

	var data []byte
	shdata, err := is.Header.MarshalBinary()
	if err != nil {
		return nil, err
	}

	data = append(data, shdata...)
	data = append(data, byte(is.Keytype))

	var bint [8]byte
	//store the keydata size
	i := binary.PutVarint(bint[:], int64(len(is.Keydata)))
	data = append(data, bint[:i]...)
	//store the key data
	data = append(data, is.Keydata[:]...)

	return data, nil
}

func (is *IdentityState) UnmarshalBinary(data []byte) error {
	dlen := len(data)
	if dlen == 0 {
		return fmt.Errorf("cannot unmarshal Identity State, insuffient data")
	}
	i := 0
	err := is.Header.UnmarshalBinary(data)
	if err != nil {
		return err
	}
	i += is.Header.GetHeaderSize()

	is.Keytype = KeyType(data[i])
	i++
	if dlen <= i {
		return fmt.Errorf("cannot unmarshal Identity State after key type, insuffient data")
	}

	v, l := binary.Varint(data[i:])
	if l <= 0 {
		return fmt.Errorf("cannot unmarshal Identity State after key data len, insuffient data")
	}
	i += l

	if dlen < i {
		return fmt.Errorf("cannot unmarshal Identity State before copy key data, insuffient data")
	}
	is.Keydata = make([]byte, v)
	i += copy(is.Keydata, data[i:i+int(v)])

	return nil
}

func (is *IdentityState) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &is.identityState)
}

func (is *IdentityState) MarshalJSON() ([]byte, error) {
	return json.Marshal(&is.identityState)
}

func (k *KeyType) UnmarshalJSON(b []byte) error {
	str := strings.Trim(string(b), `"`)

	switch {
	case str == "public":
		*k = KeyTypePublic
	case str == "sha256":
		*k = KeyTypeSha256
	case str == "sha256d":
		*k = KeyTypeSha256d
	case str == "chain":
		*k = KeyTypeChain
	case str == "group":
		*k = KeyTypeGroup
	default:
		*k = KeyTypeUnknown
	}

	return nil
}

func (k *KeyType) MarshalJSON() ([]byte, error) {
	var str string
	switch *k {
	case KeyTypePublic:
		str = "public"
	case KeyTypeSha256:
		str = "sha256"
	case KeyTypeSha256d:
		str = "sha256d"
	case KeyTypeChain:
		str = "chain"
	case KeyTypeGroup:
		str = "group"
	default:
		str = "unknown"
	}

	data, _ := json.Marshal(str)
	return data, nil
}
