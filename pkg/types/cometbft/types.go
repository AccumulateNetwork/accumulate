package cometbft

import (
	"io"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"github.com/cometbft/cometbft/types"
	"github.com/cosmos/gogoproto/proto"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

//go:generate go run gitlab.com/accumulatenetwork/core/schema/cmd/generate schema schema.yml -w schema_gen.go
//go:generate go run gitlab.com/accumulatenetwork/core/schema/cmd/generate types schema.yml -w types_gen.go
//go:generate go run github.com/rinchsan/gosimports/cmd/gosimports -w .

func (v *GenesisDoc) UnmarshalBinaryFrom(rd io.Reader) error {
	dec := binary.NewDecoder(rd)
	return v.UnmarshalBinaryV2(dec)
}

type ConsensusParams types.ConsensusParams
type Block types.Block

func (c *ConsensusParams) ToProto() cmtproto.ConsensusParams {
	return (*types.ConsensusParams)(c).ToProto()
}

func (c *ConsensusParams) FromProto(d cmtproto.ConsensusParams) {
	*(*types.ConsensusParams)(c) = types.ConsensusParamsFromProto(d)
}

func (c *ConsensusParams) Copy() *ConsensusParams {
	return copyProto[cmtproto.ConsensusParams](c)
}

func (c *ConsensusParams) Equal(d *ConsensusParams) bool {
	return c == d || equalProto(c, d)
}

func (c *ConsensusParams) MarshalBinary() ([]byte, error) {
	return marshalProto(c)
}

func (c *ConsensusParams) UnmarshalBinary(b []byte) error {
	return unmarshalProto[cmtproto.ConsensusParams](c, b)
}

func (c *ConsensusParams) UnmarshalBinaryFrom(rd io.Reader) error {
	return unmarshalProtoFrom[cmtproto.ConsensusParams](c, rd)
}

func (b *Block) ToProto() cmtproto.Block {
	c, err := (*types.Block)(b).ToProto()
	if err != nil {
		panic(err)
	}
	return *c
}

func (b *Block) FromProto(c cmtproto.Block) {
	d, err := types.BlockFromProto(&c)
	if err != nil {
		panic(err)
	}
	*(*types.Block)(b) = *d
}

func (b *Block) Copy() *Block {
	return copyProto[cmtproto.Block](b)
}

func (b *Block) Equal(c *Block) bool {
	return equalProto(b, c)
}

func (b *Block) MarshalBinary() ([]byte, error) {
	return marshalProto(b)
}

func (b *Block) UnmarshalBinary(d []byte) error {
	return unmarshalProto[cmtproto.Block](b, d)
}

func (b *Block) UnmarshalBinaryFrom(rd io.Reader) error {
	return unmarshalProtoFrom[cmtproto.Block](b, rd)
}

type protoType[B any, C protoMessage[B]] interface {
	ToProto() B
	FromProto(B)
}

type protoTypePtr[A, B any, C protoMessage[B]] interface {
	*A
	protoType[B, C]
}

type protoMessage[B any] interface {
	*B
	proto.Message
}

func copyProto[B any, A any, PA protoTypePtr[A, B, C], C protoMessage[B]](a PA) PA {
	b := PA(new(A))
	b.FromProto(a.ToProto())
	return b
}

func equalProto[B any, A protoType[B, C], C protoMessage[B]](a, b A) bool {
	u, v := a.ToProto(), b.ToProto()
	return proto.Equal(C(&u), C(&v))
}

func marshalProto[A protoType[B, C], B any, C protoMessage[B]](v A) ([]byte, error) {
	u := v.ToProto()
	return proto.Marshal(C(&u))
}

func unmarshalProto[B any, A any, PA protoTypePtr[A, B, C], C protoMessage[B]](a PA, b []byte) error {
	v := C(new(B))
	err := proto.Unmarshal(b, v)
	if err != nil {
		return err
	}
	a.FromProto(*v)
	return nil
}

func unmarshalProtoFrom[B any, A any, PA protoTypePtr[A, B, C], C protoMessage[B]](a PA, rd io.Reader) error {
	b, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return unmarshalProto[B, A, PA, C](a, b)
}
