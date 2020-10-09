package validator


import (
	"fmt"
	//"github.com/FactomProject/factomd/common/factoid"
	factom "github.com/Factom-Asset-Tokens/factom"
)

type FactoidValidator struct{ValidatorInterface}

func (v *FactoidValidator) Validate(tx []byte) uint32 {
	//if pass then send to accumulator.
	//make sure the factoid transaction format is valid.

    //t := new(factoid.Transaction)
	t := new(factom.Transaction)
	err := t.UnmarshalBinary(tx)

	if err != nil {
		fmt.Printf("Invalid Transaction")
	}

	//make sure there is enough available balance
//    inp := t.GetInputs()
    //inp returns a transaction interface.

	//if so, record transaction in factoid database, and pass to validator
	return 0
}

func NewFactoidValidator() *FactoidValidator {
	v := FactoidValidator{}
	return &v
}
