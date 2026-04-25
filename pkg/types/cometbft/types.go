// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.
package cometbft

import (
	"bytes"
	"io"
	"time"

	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
	"google.golang.org/protobuf/encoding/protowire"
)

//go:generate go run gitlab.com/accumulatenetwork/core/schema/cmd/generate schema schema.yml -w schema_gen.go
//go:generate go run gitlab.com/accumulatenetwork/core/schema/cmd/generate types schema.yml -w types_gen.go
//go:generate go run github.com/rinchsan/gosimports/cmd/gosimports -w .

func (v *GenesisDoc) UnmarshalBinaryFrom(rd io.Reader) error {
	dec := binary.NewDecoder(rd)
	return v.UnmarshalBinaryV2(dec)
}

// ConsensusParams contains consensus critical parameters that determine the
// validity of blocks. Field layout and protobuf encoding match CometBFT
// v0.38 types.ConsensusParams exactly to preserve snapshot wire format.
type ConsensusParams struct {
	Block     BlockParams
	Evidence  EvidenceParams
	Validator ValidatorParams
	Version   VersionParams
	ABCI      ABCIParams
}

type BlockParams struct {
	MaxBytes int64
	MaxGas   int64
}

type EvidenceParams struct {
	MaxAgeNumBlocks int64
	MaxAgeDuration  time.Duration
	MaxBytes        int64
}

type ValidatorParams struct {
	PubKeyTypes []string
}

type VersionParams struct {
	App uint64
}

type ABCIParams struct {
	VoteExtensionsEnableHeight int64
}

// DefaultConsensusParams returns defaults matching CometBFT v0.38.
func DefaultConsensusParams() *ConsensusParams {
	return &ConsensusParams{
		Block: BlockParams{
			MaxBytes: 22020096, // 21MB
			MaxGas:   -1,
		},
		Evidence: EvidenceParams{
			MaxAgeNumBlocks: 100000,
			MaxAgeDuration:  48 * time.Hour,
			MaxBytes:        1048576, // 1MB
		},
		Validator: ValidatorParams{
			PubKeyTypes: []string{"ed25519"},
		},
	}
}

func (c *ConsensusParams) Copy() *ConsensusParams {
	if c == nil {
		return nil
	}
	cp := *c
	if c.Validator.PubKeyTypes != nil {
		cp.Validator.PubKeyTypes = make([]string, len(c.Validator.PubKeyTypes))
		copy(cp.Validator.PubKeyTypes, c.Validator.PubKeyTypes)
	}
	return &cp
}

func (c *ConsensusParams) Equal(d *ConsensusParams) bool {
	if c == d {
		return true
	}
	if c == nil || d == nil {
		return false
	}
	a, err := c.MarshalBinary()
	if err != nil {
		return false
	}
	b, err := d.MarshalBinary()
	if err != nil {
		return false
	}
	return bytes.Equal(a, b)
}

// MarshalBinary encodes ConsensusParams to protobuf wire format matching
// CometBFT's cmtproto.ConsensusParams exactly. CometBFT proto3 uses
// pointer sub-messages and emits `tag, len=N` for every non-nil sub-field
// (including empty ones, encoded as `tag, len=0`). Our struct uses value
// sub-fields, so emit a tag for every sub-field unconditionally — this
// matches CometBFT's behavior whenever sub-fields are populated, which is
// always the case after ValidateAndComplete runs against a genesis.
func (c *ConsensusParams) MarshalBinary() ([]byte, error) {
	var buf []byte

	// Field 1: block (BlockParams)
	buf = protowire.AppendTag(buf, 1, protowire.BytesType)
	buf = protowire.AppendBytes(buf, marshalBlockParams(&c.Block))

	// Field 2: evidence (EvidenceParams)
	buf = protowire.AppendTag(buf, 2, protowire.BytesType)
	buf = protowire.AppendBytes(buf, marshalEvidenceParams(&c.Evidence))

	// Field 3: validator (ValidatorParams)
	buf = protowire.AppendTag(buf, 3, protowire.BytesType)
	buf = protowire.AppendBytes(buf, marshalValidatorParams(&c.Validator))

	// Field 4: version (VersionParams)
	buf = protowire.AppendTag(buf, 4, protowire.BytesType)
	buf = protowire.AppendBytes(buf, marshalVersionParams(&c.Version))

	// Field 5: abci (ABCIParams)
	buf = protowire.AppendTag(buf, 5, protowire.BytesType)
	buf = protowire.AppendBytes(buf, marshalABCIParams(&c.ABCI))

	return buf, nil
}

// UnmarshalBinary decodes ConsensusParams from protobuf wire format.
func (c *ConsensusParams) UnmarshalBinary(b []byte) error {
	*c = ConsensusParams{}
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]

		switch typ {
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
			switch num {
			case 1:
				unmarshalBlockParams(&c.Block, v)
			case 2:
				unmarshalEvidenceParams(&c.Evidence, v)
			case 3:
				unmarshalValidatorParams(&c.Validator, v)
			case 4:
				unmarshalVersionParams(&c.Version, v)
			case 5:
				unmarshalABCIParams(&c.ABCI, v)
			}
		case protowire.VarintType:
			_, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
		case protowire.Fixed32Type:
			_, n := protowire.ConsumeFixed32(b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
		case protowire.Fixed64Type:
			_, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return nil
}

func (c *ConsensusParams) UnmarshalBinaryFrom(rd io.Reader) error {
	b, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return c.UnmarshalBinary(b)
}

// BlockParams protobuf: field 1 = max_bytes (varint), field 2 = max_gas (varint)
func marshalBlockParams(p *BlockParams) []byte {
	var buf []byte
	if p.MaxBytes != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(p.MaxBytes))
	}
	if p.MaxGas != 0 {
		buf = protowire.AppendTag(buf, 2, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(p.MaxGas))
	}
	return buf
}

func unmarshalBlockParams(p *BlockParams, b []byte) {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return
		}
		b = b[n:]
		if typ == protowire.VarintType {
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return
			}
			b = b[n:]
			switch num {
			case 1:
				p.MaxBytes = int64(v)
			case 2:
				p.MaxGas = int64(v)
			}
		} else {
			skipField(&b, typ)
		}
	}
}

// EvidenceParams protobuf: field 1 = max_age_num_blocks (varint),
// field 2 = max_age_duration (google.protobuf.Duration), field 3 = max_bytes (varint)
func marshalEvidenceParams(p *EvidenceParams) []byte {
	var buf []byte
	if p.MaxAgeNumBlocks != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(p.MaxAgeNumBlocks))
	}
	if p.MaxAgeDuration != 0 {
		durBytes := marshalDuration(p.MaxAgeDuration)
		buf = protowire.AppendTag(buf, 2, protowire.BytesType)
		buf = protowire.AppendBytes(buf, durBytes)
	}
	if p.MaxBytes != 0 {
		buf = protowire.AppendTag(buf, 3, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(p.MaxBytes))
	}
	return buf
}

func unmarshalEvidenceParams(p *EvidenceParams, b []byte) {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return
		}
		b = b[n:]
		switch typ {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return
			}
			b = b[n:]
			switch num {
			case 1:
				p.MaxAgeNumBlocks = int64(v)
			case 3:
				p.MaxBytes = int64(v)
			}
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return
			}
			b = b[n:]
			if num == 2 {
				p.MaxAgeDuration = unmarshalDuration(v)
			}
		default:
			skipField(&b, typ)
		}
	}
}

// ValidatorParams protobuf: field 1 = pub_key_types (repeated string)
func marshalValidatorParams(p *ValidatorParams) []byte {
	var buf []byte
	for _, s := range p.PubKeyTypes {
		buf = protowire.AppendTag(buf, 1, protowire.BytesType)
		buf = protowire.AppendString(buf, s)
	}
	return buf
}

func unmarshalValidatorParams(p *ValidatorParams, b []byte) {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return
		}
		b = b[n:]
		if typ == protowire.BytesType {
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return
			}
			b = b[n:]
			if num == 1 {
				p.PubKeyTypes = append(p.PubKeyTypes, string(v))
			}
		} else {
			skipField(&b, typ)
		}
	}
}

// VersionParams protobuf: field 1 = app (uint64 varint)
func marshalVersionParams(p *VersionParams) []byte {
	var buf []byte
	if p.App != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, p.App)
	}
	return buf
}

func unmarshalVersionParams(p *VersionParams, b []byte) {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return
		}
		b = b[n:]
		if typ == protowire.VarintType {
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return
			}
			b = b[n:]
			if num == 1 {
				p.App = v
			}
		} else {
			skipField(&b, typ)
		}
	}
}

// ABCIParams protobuf: field 1 = vote_extensions_enable_height (int64 varint)
func marshalABCIParams(p *ABCIParams) []byte {
	var buf []byte
	if p.VoteExtensionsEnableHeight != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(p.VoteExtensionsEnableHeight))
	}
	return buf
}

func unmarshalABCIParams(p *ABCIParams, b []byte) {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return
		}
		b = b[n:]
		if typ == protowire.VarintType {
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return
			}
			b = b[n:]
			if num == 1 {
				p.VoteExtensionsEnableHeight = int64(v)
			}
		} else {
			skipField(&b, typ)
		}
	}
}

// google.protobuf.Duration: field 1 = seconds (int64), field 2 = nanos (int32)
func marshalDuration(d time.Duration) []byte {
	secs := int64(d / time.Second)
	nanos := int32(d % time.Second)
	var buf []byte
	if secs != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(secs))
	}
	if nanos != 0 {
		buf = protowire.AppendTag(buf, 2, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(nanos))
	}
	return buf
}

func unmarshalDuration(b []byte) time.Duration {
	var secs int64
	var nanos int32
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return 0
		}
		b = b[n:]
		if typ == protowire.VarintType {
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return 0
			}
			b = b[n:]
			switch num {
			case 1:
				secs = int64(v)
			case 2:
				nanos = int32(v)
			}
		} else {
			skipField(&b, typ)
		}
	}
	return time.Duration(secs)*time.Second + time.Duration(nanos)
}

// google.protobuf.Timestamp: field 1 = seconds (int64), field 2 = nanos (int32)
func marshalTimestamp(t time.Time) []byte {
	secs := t.Unix()
	nanos := int32(t.Nanosecond())
	var buf []byte
	if secs != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(secs))
	}
	if nanos != 0 {
		buf = protowire.AppendTag(buf, 2, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(nanos))
	}
	return buf
}

func unmarshalTimestamp(b []byte) time.Time {
	var secs int64
	var nanos int32
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return time.Time{}
		}
		b = b[n:]
		if typ == protowire.VarintType {
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return time.Time{}
			}
			b = b[n:]
			switch num {
			case 1:
				secs = int64(v)
			case 2:
				nanos = int32(v)
			}
		} else {
			skipField(&b, typ)
		}
	}
	return time.Unix(secs, int64(nanos)).UTC()
}

// skipField advances b past one field value of the given wire type.
func skipField(b *[]byte, typ protowire.Type) {
	switch typ {
	case protowire.VarintType:
		_, n := protowire.ConsumeVarint(*b)
		if n < 0 {
			*b = nil
			return
		}
		*b = (*b)[n:]
	case protowire.Fixed32Type:
		_, n := protowire.ConsumeFixed32(*b)
		if n < 0 {
			*b = nil
			return
		}
		*b = (*b)[n:]
	case protowire.Fixed64Type:
		_, n := protowire.ConsumeFixed64(*b)
		if n < 0 {
			*b = nil
			return
		}
		*b = (*b)[n:]
	case protowire.BytesType:
		_, n := protowire.ConsumeBytes(*b)
		if n < 0 {
			*b = nil
			return
		}
		*b = (*b)[n:]
	default:
		*b = nil
	}
}

// Block stores a CometBFT block. Uses raw protobuf bytes for lossless
// round-trip serialization while exposing only the header fields we need.
type Block struct {
	Header

	// rawProto stores the original protobuf bytes for round-trip fidelity.
	// If set, MarshalBinary returns these bytes verbatim.
	rawProto []byte
}

// Header contains the subset of CometBFT block header fields that
// the Accumulate codebase actually uses.
type Header struct {
	ChainID string
	Height  int64
	Time    time.Time
}

func (b *Block) Copy() *Block {
	if b == nil {
		return nil
	}
	cp := &Block{
		Header: b.Header,
	}
	if b.rawProto != nil {
		cp.rawProto = make([]byte, len(b.rawProto))
		copy(cp.rawProto, b.rawProto)
	}
	return cp
}

func (b *Block) Equal(c *Block) bool {
	if b == c {
		return true
	}
	if b == nil || c == nil {
		return false
	}
	a1, _ := b.MarshalBinary()
	a2, _ := c.MarshalBinary()
	return bytes.Equal(a1, a2)
}

// MarshalBinary returns the protobuf encoding. If the Block was decoded from
// protobuf bytes, returns those bytes verbatim (lossless round-trip).
func (b *Block) MarshalBinary() ([]byte, error) {
	if b.rawProto != nil {
		return b.rawProto, nil
	}
	// Encode just the header fields we have.
	// Proto Block: field 1 = header (message)
	headerBytes := marshalBlockHeader(&b.Header)
	if len(headerBytes) == 0 {
		return nil, nil
	}
	var buf []byte
	buf = protowire.AppendTag(buf, 1, protowire.BytesType)
	buf = protowire.AppendBytes(buf, headerBytes)
	return buf, nil
}

// marshalBlockHeader encodes the Header fields we track.
// Proto Header: field 2 = chain_id (string), field 3 = height (varint),
// field 4 = time (google.protobuf.Timestamp)
func marshalBlockHeader(h *Header) []byte {
	var buf []byte
	if h.ChainID != "" {
		buf = protowire.AppendTag(buf, 2, protowire.BytesType)
		buf = protowire.AppendString(buf, h.ChainID)
	}
	if h.Height != 0 {
		buf = protowire.AppendTag(buf, 3, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(h.Height))
	}
	if !h.Time.IsZero() {
		ts := marshalTimestamp(h.Time)
		buf = protowire.AppendTag(buf, 4, protowire.BytesType)
		buf = protowire.AppendBytes(buf, ts)
	}
	return buf
}

// UnmarshalBinary decodes a Block from protobuf bytes. Stores the raw
// bytes for lossless round-trip and extracts header fields we use.
func (b *Block) UnmarshalBinary(d []byte) error {
	*b = Block{}
	b.rawProto = make([]byte, len(d))
	copy(b.rawProto, d)

	// Parse just the header (field 1) to extract ChainID, Height, Time
	data := d
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return protowire.ParseError(n)
		}
		data = data[n:]

		if num == 1 && typ == protowire.BytesType {
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return protowire.ParseError(n)
			}
			data = data[n:]
			unmarshalBlockHeader(&b.Header, v)
		} else {
			switch typ {
			case protowire.VarintType:
				_, n := protowire.ConsumeVarint(data)
				if n < 0 {
					return protowire.ParseError(n)
				}
				data = data[n:]
			case protowire.BytesType:
				_, n := protowire.ConsumeBytes(data)
				if n < 0 {
					return protowire.ParseError(n)
				}
				data = data[n:]
			case protowire.Fixed32Type:
				_, n := protowire.ConsumeFixed32(data)
				if n < 0 {
					return protowire.ParseError(n)
				}
				data = data[n:]
			case protowire.Fixed64Type:
				_, n := protowire.ConsumeFixed64(data)
				if n < 0 {
					return protowire.ParseError(n)
				}
				data = data[n:]
			}
		}
	}
	return nil
}

func (b *Block) UnmarshalBinaryFrom(rd io.Reader) error {
	d, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return b.UnmarshalBinary(d)
}

// unmarshalBlockHeader extracts ChainID, Height, Time from header proto bytes.
func unmarshalBlockHeader(h *Header, b []byte) {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return
		}
		b = b[n:]
		switch typ {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return
			}
			b = b[n:]
			if num == 3 {
				h.Height = int64(v)
			}
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return
			}
			b = b[n:]
			switch num {
			case 2:
				h.ChainID = string(v)
			case 4:
				h.Time = unmarshalTimestamp(v)
			}
		default:
			skipField(&b, typ)
		}
	}
}
