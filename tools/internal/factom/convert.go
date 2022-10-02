// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package factom

import (
	"github.com/FactomProject/factomd/common/interfaces"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func ConvertEntry(in interfaces.IEntry) *protocol.FactomDataEntry {
	out := new(protocol.FactomDataEntry)
	out.AccountId = in.GetChainID().Fixed()
	out.Data = in.GetContent()
	out.ExtIds = in.ExternalIDs()
	return out
}
