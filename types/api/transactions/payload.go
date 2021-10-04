package transactions

import (
	"encoding"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
)

type Payload interface {
	Type() types.TxType
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

var payloads = map[types.TxType]func() Payload{}

func RegisterPayload(fn func() Payload) {
	payloads[fn().Type()] = fn
}

func MarshalType(typ types.TxType) []byte {
	return common.Uint64Bytes(typ.AsUint64())
}

func UnmarshalType(b []byte) (types.TxType, []byte) {
	typ, b := common.BytesUint64(b)
	return types.TxType(typ), b
}

func marshalPayload(p Payload) ([]byte, error) {
	if _, ok := payloads[p.Type()]; !ok {
		return nil, fmt.Errorf("unregistered transaction type %d", p.Type())
	}

	return p.MarshalBinary()
}

func unmarshalPayload(b []byte) (Payload, error) {
	typ, _ := UnmarshalType(b)
	fn, ok := payloads[typ]
	if !ok {
		return nil, fmt.Errorf("unknown transaction type %d", typ)
	}

	p := fn()
	err := p.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}

	return p, nil
}
