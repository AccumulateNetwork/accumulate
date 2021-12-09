package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type SyntheticMirror struct{}

func (SyntheticMirror) Type() types.TxType { return types.TxTypeSyntheticMirror }

func (SyntheticMirror) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.SyntheticMirror)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	for _, obj := range body.Objects {
		// TODO Check merkle tree

		// Unmarshal the record
		record, err := unmarshalRecord(obj)
		if err != nil {
			return fmt.Errorf("failed to unmarshal record: %v", err)
		}

		// Ensure the URL is valid
		_, err = record.Header().ParseUrl()
		if err != nil {
			return fmt.Errorf("invalid chain URL: %v", record.Header().ChainUrl)
		}

		// TODO Save the merkle state somewhere?
		fmt.Printf("Mirrored %q\n", record.Header().ChainUrl)
		st.Update(record)
	}

	return nil
}
