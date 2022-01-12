
package protocol





func (c CreateValidator) SetValidator(val ValidatorType) {
	
	c.ValidatorAddress = val.OperatorAddress
	c.PubKey = val.ConsensusPubKey
	
	//cvv.Commission = val.Commission

}


func (c CreateValidator) SetValidatorByAddr(val ValidatorType) error {
	pK, err := val.GetConsensusAddress()
	if err != nil {
		panic(err)
	}
	c.ValidatorAddress = val.OperatorAddress
	c.PubKey = pK
	return nil
	//cvv.Commission = val.Commission

}

func (c CreateValidator) SetValidatorByPower(val ValidatorType)  {
	if val.Jailed {
		return 
	}

}


func (c CreateValidator) SetNewValidatorByPower(val ValidatorType) {
//	c.GetValidatorByPower(val)
	
	//cvv.Commission = val.Commission

}