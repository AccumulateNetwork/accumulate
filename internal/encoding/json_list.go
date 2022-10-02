// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"encoding/json"
)

// JsonUnmarshalListWith combines the functionality of JsonList and
// JsonUnmarshalWith, using the given function to unmarshal JSON into a slice of
// T and falls back to unmarshalling it into a single value of type T.
type JsonUnmarshalListWith[T any] struct {
	Value []T
	Func  func([]byte) (T, error)
}

func (j *JsonUnmarshalListWith[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.Value)
}

func (j *JsonUnmarshalListWith[T]) unmarshalList(data []byte) error {
	var list JsonList[json.RawMessage]
	err := json.Unmarshal(data, &list)
	if err != nil {
		return err
	}

	j.Value = make([]T, len(list))
	for i, raw := range list {
		j.Value[i], err = j.Func(raw)
		if err != nil {
			return err
		}
	}

	return nil
}

func (j *JsonUnmarshalListWith[T]) unmarshalSingle(data []byte) error {
	v, err := j.Func(data)
	if err != nil {
		return err
	}

	j.Value = []T{v}
	return nil
}

func (j *JsonUnmarshalListWith[T]) UnmarshalJSON(data []byte) error {
	// Attempt to unmarshal as a slice
	err := j.unmarshalList(data)
	if err == nil {
		return nil
	}

	// Attempt to unmarshal as a single value
	if j.unmarshalSingle(data) == nil {
		return nil
	}

	// Return the error from unmarshalling as a slice
	return err
}

// JsonUnmarshalWith uses the given function to unmarshal JSON into a value of
// type T.
type JsonUnmarshalWith[T any] struct {
	Value T
	Func  func([]byte) (T, error)
}

func (j *JsonUnmarshalWith[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.Value)
}

func (j *JsonUnmarshalWith[T]) UnmarshalJSON(data []byte) error {
	v, err := j.Func(data)
	if err != nil {
		return err
	}

	j.Value = v
	return nil
}

// JsonList attempts to unmarshal JSON into a slice of T and falls back to
// unmarshalling it into a single value of type T. Thus, fields of type JsonList
// can be unmarshaled from a single value or a list.
type JsonList[T any] []T

func (j *JsonList[T]) UnmarshalJSON(data []byte) error {
	// Attempt to unmarshal as a slice
	var list []T
	err1 := json.Unmarshal(data, &list)
	if err1 == nil {
		*j = list
		return nil
	}

	// Attempt to unmarshal as a single value
	var single T
	err2 := json.Unmarshal(data, &single)
	if err2 == nil {
		*j = JsonList[T]{single}
		return nil
	}

	// Return the error from unmarshalling as a slice
	return err1
}
