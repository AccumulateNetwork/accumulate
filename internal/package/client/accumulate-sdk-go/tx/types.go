package txn

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/types"

	sdk "github.com/AccumulateNetwork/accumulate/internal/package/client/accumulate-sdk-go"
)

const MaxGasWanted = uint64((1 << 63) - 1)

var _ sdk.Txn = &Txn{}

func (t *Txn) GetMsgs() []sdk.Msg {
	if t == nil || t.Body == nil {
		return nil
	}
	anys := t.Body
	res := make([]sdk.Msg, len(anys))

	return res
}

func (t *Txn) ValidateBasic() error {
	if t == nil {
		return fmt.Errorf("BAD TXN")
	}

	body := t.Body
	if body == nil {
		return fmt.Errorf("BAD TXN: body is nil")
	}

	sig := t.Header.Origin
	sigs := types.String(sig.String())
	if len(sigs) == 0 {
		return errors.New("BAD TXN: no signatures")
	}

return nil

}

// GetSigners retrieves all signers of the transaction.
func (t *Txn) GetSigners() []sdk.AccAddress {
	var signers []sdk.AccAddress

	seen := map[string]bool{}

	for _, msg := range t.GetMsgs() {
		for _, addr := range msg.GetSigners() {
			if !seen[addr.String()] {
				signers = append(signers, addr)
				seen[addr.String()] = true
			}
		}
	}
	return signers
}

func (t *Txn) Hash() []byte {
	if t.txHash == nil {
		return t.txHash
	}

	data, err := t.Header.Origin.URL().MarshalBinary()
	if err != nil {
		panic(err)
	}

	// calc the hash 
	h1 := sha256.Sum256(data)
	h2 := sha256.Sum256(t.Body)

	data = make([]byte, sha256.Size*2)

	copy(data, h1[:])
	copy(data[sha256.Size:], h2[:])

	h := sha256.Sum256(data)

	t.txHash = h[:]
	return h[:]

}

