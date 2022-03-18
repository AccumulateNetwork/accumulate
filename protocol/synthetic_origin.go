package protocol

import "gitlab.com/accumulatenetwork/accumulate/internal/url"

func (so *SyntheticOrigin) SetSyntheticOrigin(txid []byte, source *url.URL) {
	so.Source = source
	copy(so.Cause[:], txid)
}
