// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"bytes"
	"io"
	"unsafe"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

const paramsV2Magic = "\xC0\xFF\xEE"

// paramsStateSize is the marshaled size of [parameters].
const paramsStateSize = 1 + 2 + 2 + 32

func (b *BPT) SetParams(p Parameters) error {
	if b.loadedState == nil {
		s, err := b.getState().Get()
		switch {
		case err == nil:
			b.loadedState = s
			s.Mask = s.Power - 1
		case errors.Is(err, errors.NotFound):
			b.loadedState = new(stateData)
		default:
			return errors.UnknownError.Wrap(err)
		}
	}

	if b.loadedState.Parameters == (Parameters{}) {
		b.loadedState.Parameters = p
		return b.storeState()
	}

	if !b.loadedState.Parameters.Equal(&p) {
		return errors.BadRequest.With("BPT parameters cannot be modified")
	}
	return nil
}

// loadState loads the state or returns the previously loaded value. If the
// state has not been populated, loadState sets it to the default.
func (b *BPT) loadState() (*stateData, error) {
	if b.loadedState != nil {
		return b.loadedState, nil
	}

	s, err := b.getState().Get()
	switch {
	case err == nil:
		b.loadedState = s
		s.Mask = s.Power - 1
		return s, nil
	case !errors.Is(err, errors.NotFound):
		return nil, errors.UnknownError.Wrap(err)
	}

	// Set defaults
	b.loadedState = new(stateData)
	err = b.storeState()
	return b.loadedState, err
}

func (b *BPT) mustLoadState() *stateData {
	s, err := b.loadState()
	if err != nil {
		panic(err)
	}
	return s
}

func (b *BPT) storeState() error {
	s := b.loadedState
	if s == nil {
		panic("state has not been loaded")
	}

	// Default power
	if s.Power == 0 {
		s.Power = 8
	}

	// Mask is always power - 1
	s.Mask = s.Power - 1

	return b.getState().Put(s)
}

func (r *stateData) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	_, _ = buf.WriteString(paramsV2Magic)
	enc := binary.NewEncoder(buf)
	err := wstateData.MarshalBinary(enc, r)
	return buf.Bytes(), err
}

func (r *stateData) UnmarshalBinary(data []byte) error {
	return r.UnmarshalBinaryFrom(bytes.NewBuffer(data))
}

func (r *stateData) UnmarshalBinaryFrom(rd io.Reader) error {
	const N = len(paramsV2Magic)
	var magic [N]byte
	_, err := io.ReadFull(rd, magic[:])
	if err != nil {
		return err
	}

	// If the first 3 bytes matches the magic value, decode using the schema
	if unsafe.String(&magic[0], N) == paramsV2Magic {
		dec := binary.NewDecoder(rd)
		return wstateData.UnmarshalBinary(dec, r)
	}

	// Otherwise decode the old way
	var buf [paramsStateSize]byte
	*(*[N]byte)(buf[:]) = magic
	_, err = io.ReadFull(rd, buf[N:])
	if err != nil {
		return err
	}

	// Unmarshal the fields
	r.MaxHeight = uint64(buf[0])
	r.Power = uint64(buf[1])<<8 + uint64(buf[2])
	r.Mask = uint64(buf[3])<<8 + uint64(buf[4])
	r.RootHash = *(*[32]byte)(buf[5:])
	return nil
}
