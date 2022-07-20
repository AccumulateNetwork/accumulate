package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/config"
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

	p.primary, err = accumulated.Load(primaryDir, func(c *config.Config) (io.Writer, error) {
		return logWriter(c.LogFormat, func(w io.Writer, format string, color bool) io.Writer {
			return newNodeWriter(w, format, "dn", 1, color)
		})
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

	p.primary, err = accumulated.Load(secondaryDir, func(c *config.Config) (io.Writer, error) {
		return logWriter(c.LogFormat, func(w io.Writer, format string, color bool) io.Writer {
			return newNodeWriter(w, format, "bvn", 1, color)
		})
	})
	if err != nil {
		return err
	}

	// Only one node can run Prometheus
	p.secondary.Config.Instrumentation.Prometheus = false

	if flagRun.EnableTimingLogs {
		p.secondary.Config.Accumulate.AnalysisLog.Enabled = true
	}

	return startDual(p.primary, p.secondary)
}

func (p *Program) Stop(service.Service) error {
	if p.secondary == nil {
		return p.primary.Stop()
	}

	return stopDual(p.primary, p.secondary)
}

func startDual(primary, secondary *accumulated.Daemon) (err error) {
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
