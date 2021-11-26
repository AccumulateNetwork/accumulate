package api

import "encoding/json"

func (m *JrpcMethods) parse(params json.RawMessage, target interface{}, validateFields ...string) error {
	err := json.Unmarshal(params, target)
	if err != nil {
		return validatorError(err)
	}

	// validate fields
	if len(validateFields) == 0 {
		if err = m.validate.Struct(target); err != nil {
			return validatorError(err)
		}
	} else {
		if err = m.validate.StructPartial(target, validateFields...); err != nil {
			return validatorError(err)
		}
	}

	return nil
}

func jrpcFormatQuery(res *QueryResponse, err error) interface{} {
	if err != nil {
		return accumulateError(err)
	}

	return res
}
