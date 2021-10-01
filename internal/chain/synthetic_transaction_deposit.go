package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
)

type SynTxDeposit struct{}

func (SynTxDeposit) chainType() types.ChainType { return types.ChainTypeUnknown }

func (SynTxDeposit) instruction() types.TxType {
	return types.TxTypeSyntheticTokenDeposit
}

func (SynTxDeposit) BeginBlock() {}

func (SynTxDeposit) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	_, _, _, err := checkSynTxDeposit(st, tx.Transaction)
	return err
}

func (SynTxDeposit) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	_, tas, ttd, err := checkSynTxDeposit(st, tx.Transaction)

	if ttd != nil && err != nil {
		//return to sender...
		rts, err2 := returnToSenderTx(ttd, tx)
		if err2 != nil {
			//shouldn't get here.
			return nil, err2
		}
		return rts, err
	}

	if err != nil {
		//the transaction data is bad and cannot credit nor return funds.
		return nil, err
	}

	//Modify the state object -> add the deposit amount to the balance
	err = tas.AddBalance(&ttd.DepositAmount)
	if err != nil {
		rts, err2 := returnToSenderTx(ttd, tx)
		if err2 != nil {
			//shouldn't get here.
			return nil, err2
		}
		return rts, err
	}

	//Tell BVC to modify the state for the chain
	//TODO: should we send back an ack tx to the sender? ret.Submissions = make([]pb.Submission, 1)

	//Marshal the state change...
	stateData, err := tas.MarshalBinary()

	//make sure marshalling went ok, if it didn't send the transaction back to sender.
	if err != nil {
		//we failed to marshal the token account state, shouldn't get here...
		rts, err2 := returnToSenderTx(ttd, tx)
		if err2 != nil {
			//shouldn't get here.
			return nil, err2
		}
		return rts, err
	}

	ret := new(DeliverTxResult)
	ret.AddStateData(types.GetChainIdFromChainPath(tas.ChainUrl.AsString()), stateData)
	return ret, nil
}

func checkSynTxDeposit(st *state.StateEntry, data []byte) (*state.AdiState, *state.TokenAccount, *synthetic.TokenTransactionDeposit, error) {
	ttd := &synthetic.TokenTransactionDeposit{}
	err := ttd.UnmarshalBinary(data)

	if err != nil {
		return nil, nil, nil, err
	}

	ids := state.AdiState{}
	err = ids.UnmarshalBinary(st.IdentityState.Entry)
	if err != nil {
		return nil, nil, nil, err
	}

	tas := state.TokenAccount{}
	err = tas.UnmarshalBinary(st.ChainState.Entry)
	if err != nil {
		return &ids, nil, ttd, err
	}

	if ttd.TokenUrl != types.String(tas.GetChainUrl()) {
		return &ids, &tas, ttd, fmt.Errorf("Invalid token Issuer identity")
	}

	if ttd.DepositAmount.Sign() <= 0 {
		return &ids, &tas, ttd, fmt.Errorf("Deposit must be a positive amount")
	}

	return &ids, &tas, ttd, nil
}

func returnToSenderTx(ttd *synthetic.TokenTransactionDeposit, submission *transactions.GenTransaction) (*DeliverTxResult, error) {
	res := new(DeliverTxResult)
	res.Submissions = make([]*transactions.GenTransaction, 1)
	res.Submissions[0] = &transactions.GenTransaction{}
	rs := res.Submissions[0]
	rs.Routing = types.GetAddressFromIdentity(ttd.FromUrl.AsString())
	rs.ChainID = types.GetChainIdFromChainPath(ttd.FromUrl.AsString()).Bytes()
	//this will reverse the deposit and send it back to the sender.
	retdep := synthetic.NewTokenTransactionDeposit(ttd.Txid[:], &ttd.ToUrl, &ttd.FromUrl)
	retdep.TokenUrl = ttd.TokenUrl
	err := retdep.Metadata.UnmarshalJSON([]byte("{\"deposit failed\"}"))
	if err != nil {
		return nil, err
	}
	retdep.DepositAmount.Set(&ttd.DepositAmount)
	retdepdata, err := retdep.MarshalBinary()
	if err != nil {
		//shouldn't get here.
		return nil, err
	}
	rs.Transaction = retdepdata
	return res, nil
}
