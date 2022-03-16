package transactions

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func UnmarshalAll(data []byte) ([]*protocol.Envelope, error) {
	var envelopes []*protocol.Envelope
	rd := bytes.NewReader(data)
	for rd.Len() > 0 {
		env := new(protocol.Envelope)
		err := env.UnmarshalBinaryFrom(rd)
		if err != nil {
			return nil, err
		}

		envelopes = append(envelopes, env)
	}

	return envelopes, nil
}
