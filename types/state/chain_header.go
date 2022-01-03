package state

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"fmt"

	accenc "github.com/AccumulateNetwork/accumulate/internal/encoding"
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
	Type           types.ChainType `json:"type" form:"type" query:"type" validate:"required"`
	ChainUrl       types.String    `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	KeyBook        types.String    `json:"keyBook"`        //this is the chain id for the sig spec for the chain
	ManagerKeyBook types.String    `json:"managerKeyBook"` //this is the manager key book url for the chain
	// transient
	url *url.URL
}

func (h *ChainHeader) Header() *ChainHeader { return h }

func (h *ChainHeader) Equal(g *ChainHeader) bool {
	return h.Type == g.Type &&
		h.ChainUrl == g.ChainUrl &&
		h.KeyBook == g.KeyBook &&
		h.ManagerKeyBook == g.ManagerKeyBook
}

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
	i += binary.PutUvarint(buf[:], uint64(len(h.KeyBook)))
	i += binary.PutUvarint(buf[:], uint64(len(h.ManagerKeyBook)))
	return i + len(h.ChainUrl) + len(h.KeyBook) + len(h.ManagerKeyBook)
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

func ChainType(data []byte) (types.ChainType, error) {
	v, err := accenc.UvarintUnmarshalBinary(data)
	if err != nil {
		return 0, err
	}

	return types.ChainType(v), nil
}

//MarshalBinary serializes the header
func (h *ChainHeader) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(common.Uint64Bytes(h.Type.ID()))
	buffer.Write(common.SliceBytes([]byte(h.ChainUrl)))
	buffer.Write(common.SliceBytes([]byte(h.KeyBook)))
	buffer.Write(common.SliceBytes([]byte(h.ManagerKeyBook)))

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

	u, data := common.BytesSlice(data)
	h.ChainUrl = types.String(u)

	spec, data := common.BytesSlice(data)
	h.KeyBook = types.String(spec)

	mgr, _ := common.BytesSlice(data)
	h.ManagerKeyBook = types.String(mgr)

	return nil
}
