package protocol

type WrappedTxPayload struct {
	TransactionPayload
}

func (w *WrappedTxPayload) UnmarshalBinary(data []byte) error {
	var typ TransactionType
	err := typ.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	pl, err := NewTransaction(typ)
	if err != nil {
		return err
	}

	err = pl.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	w.TransactionPayload = pl
	return nil
}
