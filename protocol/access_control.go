package protocol

// RequireAuthorization returns true if the transaction type always requires
// authorization even if the account allows unauthorized signers.
func (typ TransactionType) RequireAuthorization() bool {
	switch typ {
	case TransactionTypeCreateKeyPage,
		TransactionTypeUpdateKeyPage,
		TransactionTypeUpdateAccountAuth:
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
