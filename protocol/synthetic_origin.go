// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import "gitlab.com/accumulatenetwork/accumulate/pkg/url"

type SyntheticTransaction interface {
	SynthTxnWithOrigin
	TransactionBody
}

type SynthTxnWithOrigin interface {
	GetCause() (cause [32]byte, source *url.URL)
	SetCause(cause [32]byte, source *url.URL)
	GetRefund() (initiator *url.URL, refund Fee)
	SetRefund(initiator *url.URL, refund Fee)
	SetIndex(index uint64)
}

var _ SynthTxnWithOrigin = (*SyntheticOrigin)(nil)
var _ SynthTxnWithOrigin = (*SyntheticCreateIdentity)(nil)
var _ SynthTxnWithOrigin = (*SyntheticWriteData)(nil)
var _ SynthTxnWithOrigin = (*SyntheticDepositTokens)(nil)
var _ SynthTxnWithOrigin = (*SyntheticDepositCredits)(nil)
var _ SynthTxnWithOrigin = (*SyntheticBurnTokens)(nil)

func (so *SyntheticOrigin) Source() *url.URL {
	if so.Cause == nil {
		return nil
	}
	return so.Cause.Account()
}

func (so *SyntheticOrigin) GetCause() (cause [32]byte, source *url.URL) {
	return so.Cause.Hash(), so.Cause.Account()
}

func (so *SyntheticOrigin) SetCause(cause [32]byte, source *url.URL) {
	so.Cause = source.WithTxID(cause)
}

func (so *SyntheticOrigin) GetRefund() (initiator *url.URL, refund Fee) {
	return so.Initiator, Fee(so.FeeRefund)
}

func (so *SyntheticOrigin) SetRefund(initiator *url.URL, refund Fee) {
	so.Initiator = initiator
	so.FeeRefund = uint64(refund)
}

func (so *SyntheticOrigin) SetIndex(index uint64) {
	so.Index = index
}
