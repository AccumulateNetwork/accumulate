// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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

	fmt.Fprint(os.Stderr, "Error: at least one of --work-dir or --node is required\n")
	printUsageAndExit1(cmd, []string{})
	return "", nil // Not reached
}

func (p *Program) Run() error {
	ctx := contextForMainProcess(context.Background())

	err := p.Start()
	if err != nil {
		return err
	}

	<-ctx.Done()
	return p.Stop()
}

func (p *Program) Start() (err error) {
	logWriter := newLogWriter()

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
			return newNodeWriter(w, format, "node", 0, color)
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

	p.secondary, err = accumulated.Load(secondaryDir, func(c *config.Config) (io.Writer, error) {
		return logWriter(c.LogFormat, func(w io.Writer, format string, color bool) io.Writer {
			return newNodeWriter(w, format, "node", 1, color)
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

func (p *Program) Stop() error {
	if p.secondary == nil {
		return p.primary.Stop()
	}

	return stopDual(p.primary, p.secondary)
}

func startDual(primary, secondary *accumulated.Daemon) error {
	err := primary.StartP2P()
	if err != nil {
		return errors.UnknownError.WithFormat("start p2p: %w", err)
	}

	done := make(chan struct{})
	var ok bool
	stopOnFail := func(d *accumulated.Daemon) {
		<-done
		if !ok {
			_ = primary.Stop()
		}
	}

	errg := new(errgroup.Group)
	errg.Go(func() error {
		err := primary.Start()
		if err != nil {
			return errors.UnknownError.WithFormat("start primary: %w", err)
		}
		go stopOnFail(primary)
		return nil
	})
	errg.Go(func() error {
		err := secondary.StartSecondary(primary)
		if err != nil {
			return errors.UnknownError.WithFormat("start secondary: %w", err)
		}
		go stopOnFail(secondary)
		return nil
	})

	err = errg.Wait()
	ok = err == nil
	close(done)
	return err
}

func stopDual(primary, secondary *accumulated.Daemon) error {
	errg := new(errgroup.Group)
	errg.Go(primary.Stop)
	errg.Go(secondary.Stop)
	return errg.Wait()
}

func contextForMainProcess(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		signal.Stop(sigs)
		cancel()
	}()

	return ctx
}
