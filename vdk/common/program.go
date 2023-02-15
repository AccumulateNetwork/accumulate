package vdk

import (
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

type ServiceOptions struct {
	WorkDir         string
	LogFilename     string
	JsonLogFilename string
	methodMap       *jsonrpc2.MethodMap
}

type Program struct {
	cmd            *cobra.Command
	serviceOptions ServiceOptions
	primary        *jsonrpc2.MethodMap
}

func NewProgram(cmd *cobra.Command, options *ServiceOptions, listenAddress string) (p *Program, err error) {
	// Use a non-interactive password retriever
	PasswordRetriever = new(nonInteractiveRetriever)

	p = new(Program)
	p.cmd = cmd
	p.serviceOptions = *options
	p.primary, err = NewJrpc(Options{nil, listenAddress})
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
