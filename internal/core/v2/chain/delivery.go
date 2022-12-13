// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Delivery struct {
	core.Delivery
	State ProcessTransactionState
}

func (d *Delivery) NewChild(transaction *protocol.Transaction, signatures []protocol.Signature) *Delivery {
	e := new(Delivery)
	e.Delivery = *d.Delivery.NewChild(transaction, signatures)
	return e
}

func (d *Delivery) NewInternal(transaction *protocol.Transaction) *Delivery {
	e := new(Delivery)
	e.Delivery = *d.Delivery.NewInternal(transaction)
	return e
}

func (d *Delivery) NewForwarded(fwd *protocol.SyntheticForwardTransaction) *Delivery {
	e := new(Delivery)
	e.Delivery = *d.Delivery.NewForwarded(fwd)
	return e
}

func (d *Delivery) NewSyntheticReceipt(hash [32]byte, source *url.URL, receipt *merkle.Receipt) *Delivery {
	e := new(Delivery)
	e.Delivery = *d.Delivery.NewSyntheticReceipt(hash, source, receipt)
	return e
}

func (d *Delivery) NewSyntheticFromSequence(hash [32]byte) *Delivery {
	e := new(Delivery)
	e.Delivery = *d.Delivery.NewSyntheticFromSequence(hash)
	return e
}
