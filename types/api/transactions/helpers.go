package transactions

import (
	"bytes"
)

func UnmarshalAll(data []byte) ([]*Envelope, error) {
	var envelopes []*Envelope
	rd := bytes.NewReader(data)
	for rd.Len() > 0 {
		env := new(Envelope)
		err := env.UnmarshalBinaryFrom(rd)
		if err != nil {
			return nil, err
		}

		envelopes = append(envelopes, env)
	}

	return envelopes, nil
}
