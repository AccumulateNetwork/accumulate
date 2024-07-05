// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package jsonrpc

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Service interface {
	methods() jsonrpc2.MethodMap
}

func NewHandler(services ...Service) (http.Handler, error) {
	methods := jsonrpc2.MethodMap{}
	for _, service := range services {
		for name, method := range service.methods() {
			if _, ok := methods[name]; ok {
				return nil, errors.Conflict.WithFormat("double registered method %q", name)
			}
			methods[name] = method
		}
	}

	return jsonrpc2.HTTPRequestHandler(methods, slogger{}), nil
}

type slogger struct{}

func (slogger) Println(values ...interface{}) {
	slog.Info(fmt.Sprint(values...), "module", "api")
}

func (slogger) Printf(format string, values ...interface{}) {
	slog.Info(fmt.Sprintf(format, values...), "module", "api")
}
