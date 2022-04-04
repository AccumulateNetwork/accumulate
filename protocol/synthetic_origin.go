package protocol

import "gitlab.com/accumulatenetwork/accumulate/internal/url"

type SynthTxnWithOrigin interface {
	GetSyntheticOrigin() (cause []byte, source *url.URL)
	SetSyntheticOrigin(cause []byte, source *url.URL)
}

func (so *SyntheticOrigin) GetSyntheticOrigin() (cause []byte, source *url.URL) {
	return so.Cause[:], so.Source
}

func (so *SyntheticOrigin) SetSyntheticOrigin(cause []byte, source *url.URL) {
	if so.Source == nil { // Some calls still set this manually so don't overwrite when it's already set
		so.Source = source
		so.Cause = *(*[32]byte)(cause)
	}
}
