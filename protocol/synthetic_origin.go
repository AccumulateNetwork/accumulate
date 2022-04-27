package protocol

import "gitlab.com/accumulatenetwork/accumulate/internal/url"

type SynthTxnWithOrigin interface {
	GetSyntheticOrigin() (cause []byte, source *url.URL, fee Fee)
	SetSyntheticOrigin(cause []byte, source *url.URL, fee Fee)
}

func (so *SyntheticOrigin) GetSyntheticOrigin() (cause []byte, source *url.URL, fee Fee) {
	return so.Cause[:], so.Source, Fee(so.Fee)
}

func (so *SyntheticOrigin) SetSyntheticOrigin(cause []byte, source *url.URL, fee Fee) {
	if so.Source == nil { // Some calls still set this manually so don't overwrite when it's already set
		so.Source = source
		so.Cause = *(*[32]byte)(cause)
		so.Fee = fee.AsUInt64()
	}
}
