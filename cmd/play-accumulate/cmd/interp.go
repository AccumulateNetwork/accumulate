package cmd

import (
	"reflect"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"gitlab.com/accumulatenetwork/accumulate/cmd/play-accumulate/pkg"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func NewInterpreter(s *pkg.Session) *interp.Interpreter {
	I := interp.New(interp.Options{})

	err := I.Use(stdlib.Symbols)
	if err != nil {
		panic(err)
	}

	pkg := map[string]reflect.Value{
		"ACME": reflect.ValueOf(protocol.AcmeUrl()),
	}

	sv := reflect.ValueOf(s)
	st := sv.Type()
	for i, n := 0, st.NumMethod(); i < n; i++ {
		m := st.Method(i)
		if !m.IsExported() {
			continue
		}

		pkg[m.Name] = sv.Method(i)
	}

	err = I.Use(interp.Exports{
		"accumulate/accumulate": pkg,
		"protocol/protocol": map[string]reflect.Value{
			// Transactions
			"CreateIdentity":     reflect.ValueOf((*protocol.CreateIdentity)(nil)),
			"CreateTokenAccount": reflect.ValueOf((*protocol.CreateTokenAccount)(nil)),
			"SendTokens":         reflect.ValueOf((*protocol.SendTokens)(nil)),
			"CreateDataAccount":  reflect.ValueOf((*protocol.CreateDataAccount)(nil)),
			"WriteData":          reflect.ValueOf((*protocol.WriteData)(nil)),
			"WriteDataTo":        reflect.ValueOf((*protocol.WriteDataTo)(nil)),
			"AcmeFaucet":         reflect.ValueOf((*protocol.AcmeFaucet)(nil)),
			"CreateToken":        reflect.ValueOf((*protocol.CreateToken)(nil)),
			"IssueTokens":        reflect.ValueOf((*protocol.IssueTokens)(nil)),
			"BurnTokens":         reflect.ValueOf((*protocol.BurnTokens)(nil)),
			"CreateKeyPage":      reflect.ValueOf((*protocol.CreateKeyPage)(nil)),
			"CreateKeyBook":      reflect.ValueOf((*protocol.CreateKeyBook)(nil)),
			"AddCredits":         reflect.ValueOf((*protocol.AddCredits)(nil)),
			"UpdateKeyPage":      reflect.ValueOf((*protocol.UpdateKeyPage)(nil)),
			"AddValidator":       reflect.ValueOf((*protocol.AddValidator)(nil)),
			"RemoveValidator":    reflect.ValueOf((*protocol.RemoveValidator)(nil)),
			"UpdateValidatorKey": reflect.ValueOf((*protocol.UpdateValidatorKey)(nil)),
			"UpdateAccountAuth":  reflect.ValueOf((*protocol.UpdateAccountAuth)(nil)),
			"UpdateKey":          reflect.ValueOf((*protocol.UpdateKey)(nil)),

			// General
			"TokenRecipient": reflect.ValueOf((*protocol.TokenRecipient)(nil)),
		},
	})
	if err != nil {
		panic(err)
	}

	_, err = I.Eval(`
		import (
			. "accumulate"
			"protocol"
		)
	`)
	if err != nil {
		panic(err)
	}

	return I
}
