// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"errors"
	"fmt"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

const (
	ServiceInstall   = "install"
	ServiceReinstall = "reinstall"
	ServiceUninstall = "uninstall"
	ServiceStart     = "start"
	ServiceStop      = "stop"
)

var serviceConfig = &service.Config{
	Name:        "accumulated",
	DisplayName: "Accumulated",
	Description: cmdMain.Short,
	Arguments:   []string{"run"},
}

var cmdService = &cobra.Command{
	Use:       "service [[un|re]install|start|stop]",
	ValidArgs: []string{ServiceInstall, ServiceReinstall, ServiceUninstall, ServiceStart, ServiceStop},
	Args:      composeArgs(cobra.OnlyValidArgs, cobra.MaximumNArgs(1)),
	Run:       manageService,
}

var flagService = struct {
	UserService bool
}{}

func init() {
	cmdMain.AddCommand(cmdService)

	initRunFlags(cmdService, true)
	cmdService.Flags().StringVar(&serviceConfig.UserName, "service-user", "", "Service user name")
	cmdService.Flags().BoolVar(&flagService.UserService, "user-service", false, "Install as a user service instead of a system service")
}

func manageService(cmd *cobra.Command, args []string) {
	if len(args) > 1 {
		printUsageAndExit1(cmd, args)
	}

	if flagService.UserService {
		serviceConfig.Option["UserService"] = true
	}

	prog := NewProgram(cmd, singleNodeWorkDir, nil)
	svc, err := service.New(prog, serviceConfig)
	check(err)

	if len(args) == 0 {
		st, err := svc.Status()
		if errors.Is(err, service.ErrNotInstalled) {
			fmt.Printf("Accumulated service is not installed\n")
			return
		}
		check(err)

		switch st {
		case service.StatusRunning:
			fmt.Printf("Accumulated service is running\n")
		case service.StatusStopped:
			fmt.Printf("Accumulated service is stopped\n")
		default:
			fmt.Printf("Accumulated service status is unknown\n")
		}
	}

	switch args[0] {
	case ServiceInstall, ServiceReinstall:
		wd, err := singleNodeWorkDir(cmd)
		check(err)

		// TODO Better way of doing this?
		serviceConfig.Arguments = append(serviceConfig.Arguments, "--work-dir", wd)
	}

	switch args[0] {
	case ServiceInstall:
		check(svc.Install())
	case ServiceReinstall:
		check(svc.Uninstall())
		check(svc.Install())
	case ServiceUninstall:
		check(svc.Uninstall())
	case ServiceStart:
		check(svc.Start())
	case ServiceStop:
		check(svc.Stop())
	default:
		printUsageAndExit1(cmd, args)
	}
}
