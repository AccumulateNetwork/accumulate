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
