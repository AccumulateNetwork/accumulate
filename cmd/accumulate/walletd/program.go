package walletd

import (
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"golang.org/x/sync/errgroup"
	"io"
)

type ServiceOptions struct {
	WorkDir         string
	LogFilename     string
	JsonLogFilename string
}

type Program struct {
	cmd                 *cobra.Command
	queryServiceOptions ServiceOptions
	primary             *accumulated.Daemon
}

func NewProgram(cmd *cobra.Command, options *ServiceOptions) *Program {
	p := new(Program)
	p.cmd = cmd
	p.queryServiceOptions = *options
	return p
}

func (p *Program) Start(s service.Service) (err error) {

	primaryDir, err := p.primaryDir(p.cmd)
	if err != nil {
		return err
	}

	logWriter := NewLogWriter(s)

	p.primary, err = accumulated.Load(primaryDir, func(c *config.Config) (io.Writer, error) {
		return logWriter(c.LogFormat, nil)
	})
	if err != nil {
		return err
	}

	return startDual(p.primary, p.secondary)
}

func (p *Program) Stop(service.Service) error {
	if p.secondary == nil {
		return p.primary.Stop()
	}

	return stopDual(p.primary, p.secondary)
}

func start(primary *accumulated.Daemon) (err error) {
	var didStartPrimary, didStartSecondary bool
	errg := new(errgroup.Group)
	errg.Go(func() error {
		err := primary.Start()
		if err == nil {
			didStartPrimary = true
		}
		return err
	})
	errg.Go(func() error {
		err := secondary.Start()
		if err == nil {
			didStartSecondary = true
		}
		return err
	})

	defer func() {
		if err != nil {
			errg = new(errgroup.Group)
			if didStartPrimary {
				errg.Go(primary.Stop)
			}
			if didStartSecondary {
				errg.Go(secondary.Stop)
			}
			_ = errg.Wait()
		}
	}()

	err = errg.Wait()
	if err != nil {
		return err
	}

	return primary.ConnectDirectly(secondary)
}

func stopDual(primary, secondary *accumulated.Daemon) error {
	errg := new(errgroup.Group)
	errg.Go(primary.Stop)
	errg.Go(secondary.Stop)
	return errg.Wait()
}
