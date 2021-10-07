package state

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
)

//Chain information for the state object.  Each state object will contain a header
//that will consist of the chain type enumerator
type Chain struct {
	Type      types.ChainType `json:"type" form:"type" query:"type" validate:"required"`
	ChainUrl  types.String    `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	SigSpecId types.Bytes32   `json:"sigSpecId"` //this is the chain id for the sig spec for the chain
}

func NewChain(chainUrl types.String, chainType types.ChainType) *Chain {
	chain := &Chain{}
	chain.SetHeader(chainUrl, chainType)
	return chain
}

//SetHeader sets the data for a chain header
func (h *Chain) SetHeader(chainUrl types.String, chainType types.ChainType) {
	h.ChainUrl = chainUrl
	h.Type = chainType
}

//GetHeaderSize will return the marshalled binary size of the header.
func (h *Chain) GetHeaderSize() int {
	var buf [8]byte
	i := binary.PutUvarint(buf[:], h.Type.AsUint64())
	i += binary.PutUvarint(buf[:], uint64(len(h.ChainUrl)))
	i += binary.PutUvarint(buf[:], uint64(len(h.SigSpecId)))
	return i + len(h.ChainUrl) + len(h.SigSpecId)
}

//GetType will return the chain type
func (h *Chain) GetType() uint64 {
	return h.Type.AsUint64()
}

//GetAdiChainPath returns the url to the chain of this object
func (h *Chain) GetChainUrl() string {
	return *h.ChainUrl.AsString()
}

//MarshalBinary serializes the header
func (h *Chain) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(common.Uint64Bytes(h.Type.AsUint64()))
	buffer.Write(common.SliceBytes([]byte(h.ChainUrl)))
	buffer.Write(common.SliceBytes(h.SigSpecId[:]))

	return buffer.Bytes(), nil
}

//UnmarshalBinary deserializes the data array into the header object
func (h *Chain) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if rerr := recover(); rerr != nil {
			err = fmt.Errorf("error unmarshaling chain state header ref state %v", rerr)
		}
	}()

	chainType, data := common.BytesUint64(data)
	h.Type = types.ChainType(chainType)

	url, data := common.BytesSlice(data)
	h.ChainUrl = types.String(url)

	spec, data := common.BytesSlice(data)
	h.SigSpecId.FromBytes(spec)

	return nil
}

// LoadChain retrieves and unmarshals the specified chain.
func (db *StateDB) LoadChain(chainId []byte) (*Object, *Chain, error) {
	state, err := db.GetCurrentEntry(chainId)
	if err != nil {
		return nil, nil, err
	}

	chain := new(Chain)
	err = state.As(chain)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal chain: %v", err)
	}

	return state, chain, nil
}

// LoadChainADI retrieves and unmarshals the ADI of the chain.
func (db *StateDB) LoadChainADI(chain *Chain) (*types.Bytes32, *Object, *Chain, error) {
	adiChain := types.GetIdentityChainFromIdentity(chain.ChainUrl.AsString())
	adiState, adiHeader, err := db.LoadChain(adiChain.Bytes())
	return adiChain, adiState, adiHeader, err
}
