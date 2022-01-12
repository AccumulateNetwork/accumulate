package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
)

type CreateValidator struct{}

func (CreateValidator) Type() types.TxType { return types.TxTypeCreateValidator}

func (CreateValidator) Validate(st *StateManager, cv *protocol.CreateValidator) error {
	body := new(protocol.CreateValidator)
	

	vAddr := cv.ValidatorAddress

	if cv.Commission.CommissionRates.Rate > body.Commission.CommissionRates.MaxRate {
		return fmt.Errorf("commission rate must be less than max rate")
	}

	if _, found := types.GetValidator(vAddr); found {
		return fmt.Errorf("validator already exists")
	}
	
	pubKey := body.PubKey
	

	val, err := protocol.NewValidators(vAddr, pubKey, cv.Description)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	commish := protocol.NewCommissionRatesWithTime(
		cv.Commission.CommissionRates.Rate,
		cv.Commission.CommissionRates.MaxRate, 
		cv.Commission.CommissionRates.MaxChangeRate,
		)

	val, err = val.SetInitCommission(commish)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	body.SetValidator(val)
	body.SetValidatorByAddr(val)
	body.SetValidatorByPower(val)

	//st.Create(body)
	return nil

	//st.Create(toke)
}
