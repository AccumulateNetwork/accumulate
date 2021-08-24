package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"strings"
)

type KeyType byte

const (
	KeyTypeUnknown KeyType = iota
	KeyTypePublic
	KeyTypeSha256
	KeyTypeSha256d
	KeyTypeChain
)

type adiState struct {
	Chain
	KeyType KeyType     `json:"keyType"`
	KeyData types.Bytes `json:"keyData"`
}

type AdiState struct {
	Entry
	adiState
}

// NewIdentityState this will eventually be the key groups and potentially just a multi-map of types to chain paths controlled by the identity
func NewIdentityState(adi string) *AdiState {
	r := &AdiState{}
	r.SetHeader(types.UrlChain(adi), api.ChainTypeAdi[:])
	return r
}

func (is *AdiState) GetChainUrl() string {
	return is.Chain.GetChainUrl()
}

func (is *AdiState) GetType() *types.Bytes32 {
	return is.Chain.GetType()
}

func (is *AdiState) VerifyKey(key []byte) bool {
	//check if key is a valid public key for identity
	if key[0] == is.KeyData[0] {
		if bytes.Compare(key, is.KeyData) == 0 {
			return true
		}
	}

	//check if key is a valid sha256(key) for identity
	kh := sha256.Sum256(key)
	if kh[0] == is.KeyData[0] {
		if bytes.Compare(key, kh[:]) == 0 {
			return true
		}
	}

	//check if key is a valid sha256d(key) for identity
	kh = sha256.Sum256(kh[:])
	if kh[0] == is.KeyData[0] {
		if bytes.Compare(key, kh[:]) == 0 {
			return true
		}
	}

	return false
}

//SetKeyData currently key data is defined based upon keyType.  This will
//be replaced once a formal spec for key groups is established.
//we will also be storing references to a key group chain managed by the identity.
//a chain will most likely be just the chain paths mapped to chain types
func (is *AdiState) SetKeyData(keyType KeyType, data []byte) error {
	if len(data) > cap(is.KeyData) {
		is.KeyData = make([]byte, len(data))
	}
	is.KeyType = keyType
	copy(is.KeyData, data)

	return nil
}

//GetKeyData Currently this will just return the key information
//in the future the identity will hold links to a bunch of sub-chains
//managed by the identities.  one of them will be of key groups.
func (is *AdiState) GetKeyData() (KeyType, types.Bytes) {
	return is.KeyType, is.KeyData
}

func (is *AdiState) GetIdentityChainId() types.Bytes {
	h := types.GetIdentityChainFromIdentity(is.GetChainUrl())
	if h == nil {
		return types.Bytes{}
	}
	return h[:]
}

func (is *AdiState) MarshalBinary() ([]byte, error) {

	var data []byte
	headerData, err := is.Chain.MarshalBinary()
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	buffer.Write(headerData)
	buffer.WriteByte(byte(is.KeyType))

	var intBuf [8]byte
	//store the keyData size
	i := binary.PutVarint(intBuf[:], int64(len(is.KeyData)))
	data = append(data, intBuf[:i]...)
	//store the key data
	data = append(data, is.KeyData[:]...)

	return data, nil
}

func (is *AdiState) UnmarshalBinary(data []byte) error {
	dlen := len(data)
	if dlen == 0 {
		return fmt.Errorf("cannot unmarshal Identity State, insuffient data")
	}
	i := 0
	err := is.Chain.UnmarshalBinary(data)
	if err != nil {
		return err
	}
	i += is.Chain.GetHeaderSize()

	is.KeyType = KeyType(data[i])
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
	is.KeyData = make([]byte, v)
	i += copy(is.KeyData, data[i:i+int(v)])

	return nil
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
	default:
		str = "unknown"
	}

	data, _ := json.Marshal(str)
	return data, nil
}
