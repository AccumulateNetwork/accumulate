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

func (c *ConsensusParams) Copy() *ConsensusParams {
	d := *c
	return &d
}

func (c *ConsensusParams) Equal(d *ConsensusParams) bool {
	switch {
	case c == d:
		return true
	case c == nil,
		d == nil,
		c.Block != d.Block,
		c.Evidence != d.Evidence,
		len(c.Validator.PubKeyTypes) != len(d.Validator.PubKeyTypes),
		c.Version != d.Version,
		c.ABCI != d.ABCI:
		return false
	}
	for i := range c.Validator.PubKeyTypes {
		c, d := c.Validator.PubKeyTypes[i], d.Validator.PubKeyTypes[i]
		if c != d {
			return false
		}
	}
	return true
}

func (c *ConsensusParams) MarshalBinary() ([]byte, error) {
	d := (*types.ConsensusParams)(c).ToProto()
	return proto.Marshal(&d)
}

func (c *ConsensusParams) UnmarshalBinary(b []byte) error {
	d := new(cmtproto.ConsensusParams)
	err := proto.Unmarshal(b, d)
	if err != nil {
		return err
	}
	*c = ConsensusParams(types.ConsensusParamsFromProto(*d))
	return nil
}

func (c *ConsensusParams) UnmarshalBinaryFrom(rd io.Reader) error {
	b, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return c.UnmarshalBinary(b)
}
