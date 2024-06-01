// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"io"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cometbft/cometbft/types"
	"github.com/cosmos/gogoproto/proto"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package genesis --language go-alt types.yml

type consensusParams types.ConsensusParams

func (c *consensusParams) CopyAsInterface() any {
	d := *c
	return &d
}

func (c *consensusParams) Equal(d *consensusParams) bool {
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

func (c *consensusParams) MarshalBinary() ([]byte, error) {
	d := (*types.ConsensusParams)(c).ToProto()
	return proto.Marshal(&d)
}

func (c *consensusParams) UnmarshalBinary(b []byte) error {
	d := new(cmtproto.ConsensusParams)
	err := proto.Unmarshal(b, d)
	if err != nil {
		return err
	}
	*c = consensusParams(types.ConsensusParamsFromProto(*d))
	return nil
}

func (c *consensusParams) UnmarshalBinaryFrom(rd io.Reader) error {
	b, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return c.UnmarshalBinary(b)
}
