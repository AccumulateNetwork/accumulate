package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type SyntheticMirror struct{}

func (SyntheticMirror) Type() types.TxType { return types.TxTypeSyntheticMirror }

func (SyntheticMirror) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body := new(protocol.SyntheticMirror)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	for _, obj := range body.Objects {
		// TODO Check merkle tree

		// Unmarshal the record
		record, err := protocol.UnmarshalChain(obj.Record)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal record: %v", err)
		}

		// Ensure the URL is valid
		_, err = record.Header().ParseUrl()
		if err != nil {
			return nil, fmt.Errorf("invalid chain URL: %v", record.Header().Url)
		}

		// TODO Save the merkle state somewhere?
		st.logger.Debug("Mirroring", "url", record.Header().Url)
		st.Update(record)
	}

	return nil, nil
}
