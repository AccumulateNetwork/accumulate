package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"golang.org/x/sync/errgroup"
)

type Program struct {
	cmd                      *cobra.Command
	primaryDir, secondaryDir func(cmd *cobra.Command) (string, error)
	primary, secondary       *accumulated.Daemon
}

func NewProgram(cmd *cobra.Command, primary, secondary func(cmd *cobra.Command) (string, error)) *Program {
	p := new(Program)
	p.cmd = cmd
	p.primaryDir = primary
	p.secondaryDir = secondary
	return p
}

func singleNodeWorkDir(cmd *cobra.Command) (string, error) {
	if cmd.Flag("node").Changed {
		nodeDir := fmt.Sprintf("Node%d", flagRun.Node)
		return filepath.Join(flagMain.WorkDir, nodeDir), nil
	}

	if cmd.Flag("work-dir").Changed {
		return flagMain.WorkDir, nil
	}

	if service.Interactive() {
		fmt.Fprint(os.Stderr, "Error: at least one of --work-dir or --node is required\n")
		printUsageAndExit1(cmd, []string{})
		return "", nil // Not reached
	} else {
		return "", fmt.Errorf("at least one of --work-dir or --node is required")
	}
}

func (p *Program) Start(s service.Service) (err error) {
	logWriter := newLogWriter(s)

	primaryDir, err := p.primaryDir(p.cmd)
	if err != nil {
		return err
	}

	var secondaryDir string
	if p.secondaryDir != nil {
		secondaryDir, err = p.secondaryDir(p.cmd)
		if err != nil {
			return err
		}
	}

	p.primary, err = accumulated.Load(primaryDir, func(format string) (io.Writer, error) {
		return logWriter(format, nil)
	})
	if err != nil {
		return err
	}

	if flagRun.EnableTimingLogs {
		p.primary.Config.Accumulate.AnalysisLog.Enabled = true
	}

	if p.secondaryDir == nil {
		return p.primary.Start()
	}

	p.secondary, err = accumulated.Load(secondaryDir, func(format string) (io.Writer, error) {
		return logWriter(format, nil)
	})
	if err != nil {
		return err
	}

	// Only one node can run Prometheus
	p.secondary.Config.Instrumentation.Prometheus = false

	if flagRun.EnableTimingLogs {
		p.secondary.Config.Accumulate.AnalysisLog.Enabled = true
	}

	var didStartPrimary, didStartSecondary bool
	errg := new(errgroup.Group)
	errg.Go(func() error {
		err := p.primary.Start()
		if err == nil {
			didStartPrimary = true
		}
		return err
	})
	errg.Go(func() error {
		err := p.secondary.Start()
		if err == nil {
			didStartSecondary = true
		}
		return err
	})

	defer func() {
		if err != nil {
			errg = new(errgroup.Group)
			if didStartPrimary {
				errg.Go(p.primary.Stop)
			}
			if didStartSecondary {
				errg.Go(p.secondary.Stop)
			}
			_ = errg.Wait()
		}
	}()

	err = errg.Wait()
	if err != nil {
		return err
	}

	return p.primary.ConnectDirectly(p.secondary)
}

func (p *Program) Stop(service.Service) error {
	if p.secondary == nil {
		return p.primary.Stop()
	}

	errg := new(errgroup.Group)
	errg.Go(p.primary.Stop)
	errg.Go(p.secondary.Stop)
	return errg.Wait()
}
