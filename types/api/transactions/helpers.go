package transactions

import (
	"bytes"
	"encoding"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func New(origin string, height uint64, signer func(hash []byte) (*ED25519Sig, error), tx encoding.BinaryMarshaler) (*Envelope, error) {
	u, err := url.Parse(origin)
	if err != nil {
		return nil, fmt.Errorf("invalid origin URL: %v", err)
	}

	return NewWith(&protocol.TransactionHeader{
		Origin:        u,
		KeyPageHeight: height,
	}, signer, tx)
}

func NewWith(header *protocol.TransactionHeader, signer func(hash []byte) (*ED25519Sig, error), tx encoding.BinaryMarshaler) (*Envelope, error) {
	body, err := tx.MarshalBinary()
	if err != nil {
		return nil, err
	}

	env := new(Envelope)
	env.Transaction = new(Transaction)
	env.Transaction.TransactionHeader = *header
	env.Transaction.Body = body
	env.Signatures = make([]*ED25519Sig, 1)

	hash := env.GetTxHash()
	env.Signatures[0], err = signer(hash)
	if err != nil {
		return nil, err
	}
	return env, nil
}

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
