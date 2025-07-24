package liteclient

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
