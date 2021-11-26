package state

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/types"
)

type Chain interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	Header() *ChainHeader
}

//ChainHeader information for the state object.  Each state object will contain a header
//that will consist of the chain type enumerator
type ChainHeader struct {
	Type      types.ChainType `json:"type" form:"type" query:"type" validate:"required"`
	ChainUrl  types.String    `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	SigSpecId types.Bytes32   `json:"sigSpecId"` //this is the chain id for the sig spec for the chain
	height    uint64          `json:"height" form:"height" query:"height" validate:"required"`

	// transient
	url *url.URL
}

func (h *ChainHeader) Header() *ChainHeader { return h }
func (h *ChainHeader) GetHeight() uint64    { return h.height }

//SetHeader sets the data for a chain header
func (h *ChainHeader) SetHeader(chainUrl types.String, chainType types.ChainType) {
	h.ChainUrl = chainUrl
	h.Type = chainType
}

//GetHeaderSize will return the marshalled binary size of the header.
func (h *ChainHeader) GetHeaderSize() int {
	var buf [8]byte
	i := binary.PutUvarint(buf[:], h.Type.ID())
	i += binary.PutUvarint(buf[:], uint64(len(h.ChainUrl)))
	i += binary.PutUvarint(buf[:], uint64(len(h.SigSpecId)))
	return i + len(h.ChainUrl) + len(h.SigSpecId)
}

//GetType will return the chain type
func (h *ChainHeader) GetType() types.ChainType {
	return h.Type
}

//GetAdiChainPath returns the url to the chain of this object
func (h *ChainHeader) GetChainUrl() string {
	return *h.ChainUrl.AsString()
}

// ParseUrl returns the parsed chain URL
func (h *ChainHeader) ParseUrl() (*url.URL, error) {
	if h.url != nil {
		return h.url, nil
	}

	u, err := url.Parse(h.GetChainUrl())
	if err != nil {
		return nil, err
	}

	h.url = u
	return u, nil
}

//MarshalBinary serializes the header
func (h *ChainHeader) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(common.Uint64Bytes(h.Type.ID()))
	buffer.Write(common.SliceBytes([]byte(h.ChainUrl)))
	buffer.Write(common.SliceBytes(h.SigSpecId[:]))

	return buffer.Bytes(), nil
}

//UnmarshalBinary deserializes the data array into the header object
func (h *ChainHeader) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if rerr := recover(); rerr != nil {
			err = fmt.Errorf("error unmarshaling chain state header ref state %v", rerr)
		}
	}()

	chainType, data := common.BytesUint64(data)
	h.Type = types.ChainType(chainType)

	url, data := common.BytesSlice(data)
	h.ChainUrl = types.String(url)

	spec, _ := common.BytesSlice(data)
	h.SigSpecId.FromBytes(spec)

	height, _ := common.BytesUint64(data)
	h.height = height

	return nil
}

func (tx *DBTransaction) LoadChainAs(chainId []byte, chain Chain) (*Object, error) {
	state, err := tx.GetCurrentEntry(chainId)
	if err != nil {
		return nil, err
	}

	err = state.As(chain)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal chain: %v", err)
	}

	return state, nil
}

// LoadChain retrieves and unmarshals the specified chain.
func (tx *DBTransaction) LoadChain(chainId []byte) (*Object, *ChainHeader, error) {
	chain := new(ChainHeader)
	obj, err := tx.LoadChainAs(chainId, chain)
	return obj, chain, err
}
