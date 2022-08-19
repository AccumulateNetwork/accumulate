package factom

import (
	"github.com/FactomProject/factomd/common/entryBlock"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func ConvertEntry(in *entryBlock.Entry) *protocol.FactomDataEntry {
	out := new(protocol.FactomDataEntry)
	out.AccountId = in.ChainID.Fixed()
	out.Data = in.Content.Bytes
	out.ExtIds = in.ExternalIDs()
	return out
}
