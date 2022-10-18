package walletd

import (
	"time"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

type ServiceOptions struct {
	WorkDir         string
	LogFilename     string
	JsonLogFilename string
}

type Program struct {
	cmd            *cobra.Command
	serviceOptions ServiceOptions
	primary        *JrpcMethods
}

func NewProgram(cmd *cobra.Command, options *ServiceOptions, listenAddress string) (p *Program, err error) {
	p = new(Program)
	p.cmd = cmd
	p.serviceOptions = *options
	p.primary, err = NewJrpc(Options{nil, time.Second, listenAddress, GetWallet()})
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
