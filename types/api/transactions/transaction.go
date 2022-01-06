package transactions

import (
	"crypto/sha256"
	"encoding"
	"errors"
	"fmt"
	"strings"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types"
)

// Hash calculates the hash of the transaction as H(H(header) + H(body)).
func (t *Transaction) Hash() []byte {
	// Already computed?
	if t.txHash != nil {
		return t.txHash
	}

	// Marshal the header
	data, err := t.Header.MarshalBinary()
	if err != nil {
		// TransactionHeader.MarshalBinary will never return an error, but better safe than sorry.
		panic(err)
	}

	// Calculate the hash
	h1 := sha256.Sum256(data)
	h2 := sha256.Sum256(t.Body)
	data = make([]byte, sha256.Size*2)
	copy(data, h1[:])
	copy(data[sha256.Size:], h2[:])
	h := sha256.Sum256(data)
	t.txHash = h[:]
	return h[:]
}

// Type decodes the transaction type from the body.
func (t *Transaction) Type() types.TransactionType {
	typ := types.TxTypeUnknown
	_ = typ.UnmarshalBinary(t.Body)
	return typ
}

// Verify verifies that the signatures are valid.
func (e *Envelope) Verify() bool {
	// Compute the transaction hash
	txid := e.Transaction.Hash()

	// Check each signature
	for _, v := range e.Signatures {
		if !v.Verify(txid) {
			return false
		}
	}

	return true
}

// As unmarshals the transaction payload as the given sub transaction type.
func (e *Envelope) As(subTx encoding.BinaryUnmarshaler) error {
	return subTx.UnmarshalBinary(e.Transaction.Body)
}

func New(origin string, height uint64, signer func(hash []byte) (*ED25519Sig, error), tx encoding.BinaryMarshaler) (*Envelope, error) {
	u, err := url.Parse(origin)
	if err != nil {
		return nil, fmt.Errorf("invalid origin URL: %v", err)
	}

	return NewWith(&Header{
		Origin:        u,
		KeyPageHeight: height,
	}, signer, tx)
}

func NewWith(header *Header, signer func(hash []byte) (*ED25519Sig, error), tx encoding.BinaryMarshaler) (*Envelope, error) {
	body, err := tx.MarshalBinary()
	if err != nil {
		return nil, err
	}

	env := new(Envelope)
	env.Transaction.Header = *header
	env.Transaction.Body = body
	env.Signatures = make([]*ED25519Sig, 1)

	hash := env.Transaction.Hash()
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
