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

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

const paramsV2Magic = "\xC0\xFF\xEE"

// paramsStateSize is the marshaled size of [parameters].
const paramsStateSize = 1 + 2 + 2 + 32

// paramsRecord is a wrapper around Value that sets the power to 8 if the
// parameters have not been configured.
type paramsRecord struct {
	values.Value[*stateData]
}

// Get loads the parameters, initializing them to the default values if they
// have not been set.
func (p paramsRecord) Get() (*stateData, error) {
	v, err := p.Value.Get()
	switch {
	case err == nil:
		return v, nil
	case !errors.Is(err, errors.NotFound):
		return nil, errors.UnknownError.Wrap(err)
	}

	// TODO Allow power to be configurable?
	v = new(stateData)
	v.Power = 8
	v.Mask = v.Power - 1
	err = p.Value.Put(v)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return v, nil
}

func (r *stateData) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	_, _ = buf.WriteString(paramsV2Magic)
	enc := binary.NewEncoder(buf)
	err := wstateData.MarshalBinary(enc, &r)
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
		return wstateData.UnmarshalBinary(dec, &r)
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
