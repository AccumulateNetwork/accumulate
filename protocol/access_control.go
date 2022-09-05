package protocol

import "encoding/json"

// RequireAuthorization returns true if the transaction type always requires
// authorization even if the account allows unauthorized signers.
func (typ TransactionType) RequireAuthorization() bool {
	switch typ {
	case TransactionTypeUpdateAccountAuth:
		return true
	default:
		return false
	}
}

type AllowedTransactionBit uint8

// Mask returns the bit mask.
func (bit AllowedTransactionBit) Mask() AllowedTransactions {
	return 1 << bit
}

type AllowedTransactions uint64

// Set sets the bit to 1.
func (v *AllowedTransactions) Set(bit AllowedTransactionBit) {
	*v = *v | bit.Mask()
}

// Clear sets the bit to 0.
func (v *AllowedTransactions) Clear(bit AllowedTransactionBit) {
	*v = *v & ^bit.Mask()
}

// IsSet returns true if the bit is set.
func (v *AllowedTransactions) IsSet(bit AllowedTransactionBit) bool {
	if v == nil {
		return false
	}
	return (*v)&bit.Mask() != 0
}

// GetEnumValue implements the enumeration encoding interface.
func (v AllowedTransactions) GetEnumValue() uint64 {
	return uint64(v)
}

// SetEnumValue implements the enumeration encoding interface.
func (v *AllowedTransactions) SetEnumValue(u uint64) bool {
	*v = AllowedTransactions(u)
	return true
}

// AllowedTransactionBit returns the AllowedTransactionBit associated with this
// transaction type.
func (typ TransactionType) AllowedTransactionBit() (AllowedTransactionBit, bool) {
	switch typ {
	case TransactionTypeUpdateKeyPage:
		return AllowedTransactionBitUpdateKeyPage, true
	case TransactionTypeUpdateAccountAuth:
		return AllowedTransactionBitUpdateAccountAuth, true
	}

	return 0, false
}

// Unpack lists all of the set bits.
func (v *AllowedTransactions) Unpack() []AllowedTransactionBit {
	if v == nil {
		return nil
	}

	var bits []AllowedTransactionBit
	u := *v
	for i := 0; 1<<i <= u; i++ {
		if (1<<i)&u != 0 {
			bits = append(bits, AllowedTransactionBit(i))
		}
	}
	return bits
}

func (v *AllowedTransactions) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.Unpack())
}

func (v *AllowedTransactions) UnmarshalJSON(b []byte) error {
	var bits []AllowedTransactionBit
	err := json.Unmarshal(b, &bits)
	if err != nil {
		return err
	}

	for _, bit := range bits {
		v.Set(bit)
	}
	return nil
}
