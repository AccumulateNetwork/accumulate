package protocol

import (
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

const (
	// DefaultKeyPage is the default key page name when not specified
	DefaultKeyPage = "page0"
)

// ValidateKeyPageUrl validates whether the key page URL is sane and will return the default key page URL "<keybook parent>/page0" when not specified
func ValidateKeyPageUrl(keyBookUrl *url.URL, keyPageUrl *url.URL) (*url.URL, error) {
	bkParentUrl, err := keyBookUrl.Parent()
	if err != nil {
		return nil, fmt.Errorf("invalid KeyBook URL: %w\nthe KeyBook URL should be \"adi_path/<KeyBook>\"", err)
	}
	if keyPageUrl == nil {
		return bkParentUrl.JoinPath(DefaultKeyPage), nil
	} else {
		kpParentUrl, err2 := keyPageUrl.Parent()
		if err2 != nil {
			return nil, fmt.Errorf("invalid KeyPage URL: %w\nthe KeyPage URL should be \"adi_path/<KeyPage>\"", err)
		}

		if !bkParentUrl.Equal(kpParentUrl) {
			return nil, fmt.Errorf("KeyPage %q should have the same ADI parent path as its KeyBook %q", keyPageUrl, keyBookUrl)
		}
	}

	return keyPageUrl, nil
}
