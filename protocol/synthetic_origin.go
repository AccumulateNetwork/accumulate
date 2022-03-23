package protocol

import "gitlab.com/accumulatenetwork/accumulate/internal/url"

func (so *SyntheticOrigin) SetSyntheticOrigin(txid []byte, source *url.URL) {
	if so.Source == nil { // Some calls still set this manually so don't overwrite when it's already set
		so.Source = source
		copy(so.Cause[:], txid)
	}
}
