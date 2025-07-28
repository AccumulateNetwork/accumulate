package liteclient

import "gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"

// VerifiedAccount contains an account URL and its cryptographic proof.
type VerifiedAccount struct {
	Url     string
	Receipt *merkle.Receipt
	Height  int64
}

// Transaction represents transaction data (unified struct)
type Transaction struct {
	TxID      string
	Type      string
	Status    string
	Timestamp int64
	Amount    string
	From      string
	To        string
	Account   string
	Height    int64
	Data      interface{} // Raw transaction data
}
