package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

func (h *AccountHeader) Header() *AccountHeader { return h }

// //SetHeader sets the data for a chain header
// func (h *AccountHeader) SetHeader(chainUrl types.String, chainType types.AccountType) {
// 	h.Url = chainUrl
// 	h.Type = chainType
// }

//GetHeaderSize will return the marshalled binary size of the header.
func (h *AccountHeader) GetHeaderSize() int {
	return h.BinarySize()
}

//GetType will return the account type
func (h *AccountHeader) GetType() AccountType {
	return h.Type
}

//GetAdiChainPath returns the url to the chain of this object
func (h *AccountHeader) GetChainUrl() string {
	return h.Url
}

// ParseUrl returns the parsed chain URL
func (h *AccountHeader) ParseUrl() (*url.URL, error) {
	if h.url != nil {
		return h.url, nil
	}

	u, err := url.Parse(h.GetChainUrl())
	if err != nil {
		return nil, err
	}

	h.url = u
	return u, nil
}
