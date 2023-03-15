// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type RecordKey record.Key

func (k RecordKey) Copy() RecordKey {
	l := make(RecordKey, len(k))
	copy(l, k)
	return l
}

func (k RecordKey) Equal(l RecordKey) bool {
	if len(k) != len(l) {
		return false
	}
	for i := range k {
		if !keyPartsEqual(k[i], l[i]) {
			return false
		}
	}
	return true
}

func keyPartsEqual(v, u any) bool {
	switch v := v.(type) {
	case int64:
		_, ok := u.(int64)
		if !ok {
			return false
		}
	case uint64:
		_, ok := u.(uint64)
		if !ok {
			return false
		}
	case string:
		_, ok := u.(string)
		if !ok {
			return false
		}
	case [32]byte:
		_, ok := u.([32]byte)
		if !ok {
			return false
		}
	case *url.URL:
		u, ok := u.(*url.URL)
		if !ok {
			return false
		}
		return v.Equal(u)
	case *url.TxID:
		u, ok := u.(*url.TxID)
		if !ok {
			return false
		}
		return v.Equal(u)
	}

	return v == u
}

func (k *RecordKey) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	_, _ = buf.Write(encoding.MarshalUint(uint64(len(*k))))
	w := encoding.NewWriter(buf)

	// This is an abuse but 🤷 it works
	for _, v := range *k {
		p, err := asKeyPart(v)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		p.WriteBinary(w)
	}

	_, _, err := w.Reset(nil)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (k *RecordKey) UnmarshalBinary(b []byte) error {
	return k.UnmarshalBinaryFrom(bytes.NewBuffer(b))
}

func (k *RecordKey) UnmarshalBinaryFrom(rd io.Reader) error {
	br, ok := rd.(encoding.BytesReader)
	if !ok {
		br = bufio.NewReader(rd)
	}

	n, err := binary.ReadUvarint(br)
	if err != nil {
		return errors.UnknownError.WithFormat("decode RecordKey: %w", err)
	}
	*k = make(RecordKey, n)

	// This is an abuse but 🤷 it works
	r := encoding.NewReader(br)
	for i := uint64(0); i < n; i++ {
		v, err := binary.ReadUvarint(br)
		if err != nil {
			return errors.UnknownError.WithFormat("decode RecordKey: %w", err)
		}
		p, err := newKeyPart(TypeCode(v))
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		p.ReadBinary(r)
		(*k)[i] = p.Value()
	}

	_, err = r.Reset(nil)
	if err != nil {
		return errors.UnknownError.WithFormat("decode RecordKey: %w", err)
	}
	return nil
}

func (k RecordKey) MarshalJSON() ([]byte, error) {
	parts := make([]map[string]any, len(k))
	for i, v := range k {
		p, err := asKeyPart(v)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		parts[i] = map[string]any{p.Type().String(): p}
	}
	return json.Marshal(parts)
}

func (k *RecordKey) UnmarshalJSON(b []byte) error {
	var parts []map[string]json.RawMessage
	err := json.Unmarshal(b, &parts)
	if err != nil {
		return err
	}

	*k = make(RecordKey, len(parts))
	for i, p := range parts {
		if len(p) != 1 {
			return errors.EncodingError.WithFormat("invalid key part: expected { typ: val }, got %v", parts)
		}

		var s string
		var b json.RawMessage
		for s, b = range p {
		}

		typ, ok := TypeCodeByName(s)
		if !ok {
			return errors.EncodingError.WithFormat("%q is not a valid type code", s)
		}

		kp, err := newKeyPart(typ)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		err = json.Unmarshal(b, &kp)
		if err != nil {
			return errors.UnknownError.WithFormat("decode RecordKey: %w", err)
		}

		(*k)[i] = kp.Value()
	}
	return nil
}
