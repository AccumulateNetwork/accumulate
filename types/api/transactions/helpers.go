package transactions

import (
	"encoding"
	"errors"
	"fmt"
	"strings"

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
	var errs []string
	for len(data) > 0 {
		// Unmarshal the envelope
		env := new(Envelope)
		err := env.UnmarshalBinary(data)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}

		envelopes = append(envelopes, env)
		data = data[env.BinarySize():]
	}

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}

	return envelopes, nil
}
