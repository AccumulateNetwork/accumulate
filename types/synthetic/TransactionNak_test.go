package synthetic

import (
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/proto"
)

func TestTransactionNak(t *testing.T) {
	adichainpath := "wileecoyote/acme"
	chainid := types.GetChainIdFromChainPath(&adichainpath)
	idhash := types.GetIdentityChainFromIdentity(&adichainpath)

	sub := &proto.Submission{}
	sub.Identitychain = idhash[:]
	sub.Chainid = chainid[:]
	sub.Instruction = proto.AccInstruction_Synthetic_Transaction_Response
	sub.SourceAdiChainPath = "roadrunner/acme"

	//NewTransactionNak(NakCode_State_Change_Fail,txscript.ErrSigInvalidDataLen,)
}
