// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package jsonrpc

import (
	"fmt"
	"io"
	stdlog "log"
	"net/http"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Service interface {
	methods() jsonrpc2.MethodMap
}

func NewHandler(logger log.Logger, services ...Service) (http.Handler, error) {
	methods := jsonrpc2.MethodMap{}
	for _, service := range services {
		for name, method := range service.methods() {
			if _, ok := methods[name]; ok {
				return nil, errors.Conflict.WithFormat("double registered method %q", name)
			}
			methods[name] = method
		}
	}

	var jl jsonrpc2.Logger
	if logger == nil {
		jl = stdlog.New(io.Discard, "", 0)
	} else {
		jl = ll{logger}
	}
	return jsonrpc2.HTTPRequestHandler(methods, jl), nil
}

type ll struct {
	l log.Logger
}

func (l ll) Println(values ...interface{}) {
	l.l.Info(fmt.Sprint(values...))
}

func (l ll) Printf(format string, values ...interface{}) {
	l.l.Info(fmt.Sprintf(format, values...))
}
