package encoding

import (
	"encoding/json"
)

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
