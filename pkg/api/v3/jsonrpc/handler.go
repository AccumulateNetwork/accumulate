package jsonrpc

import (
	"fmt"
	"io"
	stdlog "log"
	"net/http"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

type Service interface {
	methods() jsonrpc2.MethodMap
}

func Handler(logger log.Logger, services ...Service) (http.Handler, error) {
	methods := jsonrpc2.MethodMap{}
	for _, service := range services {
		for name, method := range service.methods() {
			if _, ok := methods[name]; ok {
				return nil, errors.Format(errors.StatusConflict, "double registered method %q", name)
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
