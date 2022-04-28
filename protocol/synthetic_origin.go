package protocol

import "gitlab.com/accumulatenetwork/accumulate/internal/url"

type SynthTxnWithOrigin interface {
	GetCause() (cause []byte, source *url.URL)
	SetCause(cause []byte, source *url.URL)
	GetRefund() (initiator *url.URL, refund Fee)
	SetRefund(initiator *url.URL, refund Fee)
}

var _ SynthTxnWithOrigin = (*SyntheticOrigin)(nil)
var _ SynthTxnWithOrigin = (*SyntheticCreateIdentity)(nil)

func (so *SyntheticOrigin) GetCause() (cause []byte, source *url.URL) {
	return so.Cause[:], so.Source
}

func (so *SyntheticOrigin) SetCause(cause []byte, source *url.URL) {
	so.Source = source
	so.Cause = *(*[32]byte)(cause)
}

func (so *SyntheticOrigin) GetRefund() (initiator *url.URL, refund Fee) {
	return so.Initiator, Fee(so.FeeRefund)
}

func (so *SyntheticOrigin) SetRefund(initiator *url.URL, refund Fee) {
	so.Initiator = initiator
	so.FeeRefund = uint64(refund)
}
