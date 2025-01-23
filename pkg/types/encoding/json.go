// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"
)

var ErrNotEnoughData = errors.New("not enough data")
var ErrMalformedBigInt = errors.New("invalid big integer string")
var ErrOverflow = errors.New("overflow")

func BigintToJSON(b *big.Int) *string {
	if b == nil {
		return nil
	}
	s := b.String()
	return &s
}

func BytesToJSON(v []byte) *string {
	if v == nil {
		return nil
	}
	s := hex.EncodeToString(v)
	return &s
}

func ChainToJSON(v *[32]byte) *string {
	if v == nil {
		return nil
	}
	s := hex.EncodeToString(v[:])
	return &s
}

type DurationFields struct {
	Seconds     uint64 `json:"seconds,omitempty"`
	Nanoseconds uint64 `json:"nanoseconds,omitempty"`
}

func DurationToJSON(v time.Duration) interface{} {
	sec, ns := SplitDuration(v)
	return DurationFields{sec, ns}
}

func BigintFromJSON(s *string) (*big.Int, error) {
	if s == nil {
		return nil, nil
	}
	ret := new(big.Int)
	_, b := ret.SetString(*s, 10)
	if !b {
		return nil, ErrMalformedBigInt
	}
	return ret, nil
}

func BytesFromJSON(s *string) ([]byte, error) {
	if s == nil {
		return nil, nil
	}
	return hex.DecodeString(*s)
}

func ChainFromJSON(s *string) (*[32]byte, error) {
	if s == nil {
		return nil, nil
	}
	var v [32]byte
	b, err := hex.DecodeString(*s)
	if err != nil {
		return &v, err
	}
	if copy(v[:], b) < 32 {
		return &v, ErrNotEnoughData
	}
	return &v, nil
}

func DurationFromJSON(v interface{}) (time.Duration, error) {
	switch v := v.(type) {
	case float64:
		return time.Duration(v * float64(time.Second)), nil
	case int:
		return time.Second * time.Duration(v), nil
	case string:
		return time.ParseDuration(v)
	case DurationFields:
		return time.Duration(v.Seconds)*time.Second + time.Duration(v.Nanoseconds), nil
	case map[string]interface{}:
		data, err := json.Marshal(v)
		if err != nil {
			break
		}

		var df DurationFields
		if json.Unmarshal(data, &df) != nil {
			break
		}
		return time.Duration(df.Seconds)*time.Second + time.Duration(df.Nanoseconds), nil
	}
	return 0, fmt.Errorf("cannot parse %T as a duration", v)
}

func AnyToJSON(v interface{}) interface{} {
	switch v := v.(type) {
	case json.Marshaler:
		return v
	case []byte:
		return hex.EncodeToString(v)
	case [32]byte:
		return hex.EncodeToString(v[:])
	case interface{ Bytes() []byte }:
		return hex.EncodeToString(v.Bytes())
	case time.Duration:
		return DurationToJSON(v)
	default:
		return v
	}
}

// AnyFromJSON converts v to a duration if it appears to be a duration.
// AnyFromJSON never returns an error.
func AnyFromJSON(v interface{}) (interface{}, error) {
	// Include error in the return values so that the signature matches

	switch v := v.(type) {
	case map[string]interface{}:
		// Does it look like a duration?
		sec, ok1 := v["seconds"].(int)
		ns, ok2 := v["nanoseconds"].(int)
		if ok1 && ok2 && len(v) == 2 {
			return time.Duration(sec)*time.Second + time.Duration(ns), nil
		}
	}

	// There's not a lot we can do without metadata
	return v, nil
}
