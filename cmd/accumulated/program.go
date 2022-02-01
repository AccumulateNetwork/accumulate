package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/AccumulateNetwork/accumulate/internal/accumulated"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

type Program struct {
	accumulated.Daemon
	cmd *cobra.Command
}

func NewProgram(cmd *cobra.Command) *Program {
	p := new(Program)
	p.cmd = cmd
	return p
}

func (p *Program) workDir() (string, error) {
	if p.cmd.Flag("node").Changed {
		nodeDir := fmt.Sprintf("Node%d", flagRun.Node)
		return filepath.Join(flagMain.WorkDir, nodeDir), nil
	}

	if p.cmd.Flag("work-dir").Changed {
		return flagMain.WorkDir, nil
	}

	if service.Interactive() {
		fmt.Fprint(os.Stderr, "Error: at least one of --work-dir or --node is required\n")
		printUsageAndExit1(p.cmd, []string{})
		return "", nil // Not reached
	} else {
		return "", fmt.Errorf("at least one of --work-dir or --node is required")
	}
}

func (p *Program) Start(s service.Service) error {
	workDir, err := p.workDir()
	if err != nil {
		return err
	}

	logWriter := newLogWriter(s)
	daemon, err := accumulated.Load(workDir, func(format string) (io.Writer, error) {
		return logWriter(format, nil)
	})
	if err != nil {
		return err
	}
	p.Daemon = *daemon

	return p.Daemon.Start()
}

func (p *Program) Stop(service.Service) error {
	return p.Daemon.Stop()
}
