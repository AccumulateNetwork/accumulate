package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

type KeyPageOperation uint8

const (
	UpdateKey KeyPageOperation = iota + 1
	AddKey
	RemoveKey
)

func KeyPageOperationByName(s string) KeyPageOperation {
	switch strings.ToLower(s) {
	case "update":
		return UpdateKey
	case "add":
		return AddKey
	case "remove":
		return RemoveKey
	default:
		return KeyPageOperation(0)
	}
}

func (op KeyPageOperation) String() string {
	switch op {
	case UpdateKey:
		return "update"
	case AddKey:
		return "add"
	case RemoveKey:
		return "remove"
	default:
		return fmt.Sprintf("KeyPageOperation:%d", op)
	}
}

func (op KeyPageOperation) BinarySize() int {
	return 1
}

func (op KeyPageOperation) MarshalBinary() ([]byte, error) {
	return []byte{byte(op)}, nil
}

func (op *KeyPageOperation) UnmarshalBinary(b []byte) error {
	if len(b) == 0 {
		return ErrNotEnoughData
	}
	*op = KeyPageOperation(b[0])
	return nil
}

func (op KeyPageOperation) MarshalJSON() ([]byte, error) {
	return json.Marshal(op.String())
}

func (op *KeyPageOperation) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return nil
	}

	*op = KeyPageOperationByName(s)
	if *op == 0 {
		return fmt.Errorf("invalid Hash algorithm: %q", s)
	}
	return nil
}
