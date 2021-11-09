package api

import "encoding/json"

func (m *JrpcMethods) parse(params json.RawMessage, target interface{}) error {
	err := json.Unmarshal(params, target)
	if err != nil {
		return validatorError(err)
	}

	// validate URL
	err = m.validate.Struct(target)
	if err != nil {
		return validatorError(err)
	}

	return nil
}

func jrpcFormatQuery(res *QueryResponse, err error) interface{} {
	if err != nil {
		return accumulateError(err)
	}

	return res
}
