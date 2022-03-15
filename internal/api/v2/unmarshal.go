package api

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func unmarshalState(b []byte) (*protocol.Object, protocol.Account, error) {
	var obj protocol.Object
	err := obj.UnmarshalBinary(b)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid state response: %v", err)
	}

	chain, err := protocol.UnmarshalAccount(obj.Entry)
	if err != nil {
		return nil, nil, err
	}

	return &obj, chain, nil
}
