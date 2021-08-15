package synthetic

import (
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"testing"
)

func TestTransactionAckNak(t *testing.T) {
	adichainpath := "wileecoyote/acme"
	chainid := types.GetChainIdFromChainPath(adichainpath)
	idhash := types.GetIdentityChainFromIdentity(adichainpath)

	sub := &proto.Submission{}
	sub.Identitychain = idhash[:]
	sub.Chainid = chainid[:]
	sub.Instruction = proto.AccInstruction_Synthetic_Transaction_Response
	sub.SourceAdiChainPath = "roadrunner/acme"

	//NewTransactionAckNak(AckNakCode_State_Change_Fail,txscript.ErrSigInvalidDataLen,)
}
