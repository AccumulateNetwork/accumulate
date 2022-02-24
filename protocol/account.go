package protocol

import "gitlab.com/accumulatenetwork/accumulate/internal/url"

// ParseUrl returns the parsed chain URL
//
// Deprecated: use Url field
func (h *AccountHeader) ParseUrl() (*url.URL, error) {
	return h.Url, nil
}

// IsTransaction returns true if the account type is a transaction.
func (t AccountType) IsTransaction() bool {
	switch t {
	case AccountTypeTransaction, AccountTypePendingTransaction:
		return true
	default:
		return false
	}
}
