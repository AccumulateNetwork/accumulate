package vdk

import (
	"github.com/kardianos/service"
)

type ServiceOptions struct {
	WorkDir         string
	LogFilename     string
	JsonLogFilename string
	methodMap       JsonMethods
}

type Program struct {
	serviceOptions ServiceOptions
	primary        *JrpcMethods
}

func NewProgram(options *ServiceOptions, listenAddress string) (p *Program, err error) {
	// Use a non-interactive password retriever
	//PasswordRetriever = new(nonInteractiveRetriever)

	p = new(Program)
	p.serviceOptions = *options
	p.primary, err = NewJrpc(Options{nil, listenAddress, options.methodMap})
	return p, err
}

func (p *Program) Start(s service.Service) (err error) {

	logWriter := NewLogWriter(s, p.serviceOptions.LogFilename, p.serviceOptions.JsonLogFilename)
	_ = logWriter

	return p.primary.Start()
}

func (p *Program) Stop(service.Service) error {
	return p.primary.Stop()
}
