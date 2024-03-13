// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cometbft/cometbft/crypto/ed25519"
	dc "github.com/docker/cli/cli/compose/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	cfg "gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gopkg.in/yaml.v2"
)

var cmdInitDevnet = &cobra.Command{
	Use:   "devnet",
	Short: "Initialize a DevNet",
	Run:   initDevNet,
	Args:  cobra.NoArgs,
}

var baseIP net.IP
var ipCount byte

func initDevNet(cmd *cobra.Command, _ []string) {
	count := flagInitDevnet.NumValidators + flagInitDevnet.NumFollowers
	verifyInitFlags(cmd, count)

	if flagInit.Reset {
		networkReset()
	}

	initOpts := accumulated.DevnetOptions{
		BvnCount:       flagInitDevnet.NumBvns,
		ValidatorCount: flagInitDevnet.NumValidators,
		FollowerCount:  flagInitDevnet.NumFollowers,
		BsnCount:       flagInitDevnet.NumBsnNodes,
		BasePort:       flagInitDevnet.BasePort,
	}

	if !flagInitDevnet.Compose {
		initOpts.GenerateKeys = func() (privVal, dnn, bvnn, bsnn []byte) {
			return ed25519.GenPrivKey(), ed25519.GenPrivKey(), ed25519.GenPrivKey(), ed25519.GenPrivKey()
		}
	}

	if flagInitDevnet.Docker {
		initOpts.HostName = func(bvnNum, nodeNum int) (host string, listen string) {
			if bvnNum < 0 {
				return fmt.Sprintf("bsn-%d%s", nodeNum+1, flagInitDevnet.DnsSuffix), "0.0.0.0"
			}
			return fmt.Sprintf("node-%d%s", bvnNum*count+nodeNum+1, flagInitDevnet.DnsSuffix), "0.0.0.0"
		}
	} else {
		initOpts.HostName = func(int, int) (host string, listen string) {
			return nextIP(), ""
		}
	}

	netInit := accumulated.NewDevnet(initOpts)

	if flagInitDevnet.Compose {
		writeDevnetDockerCompose(cmd, netInit)
	} else {
		initNetworkLocalFS(cmd, netInit)
	}
}

func nextIP() string {
	if len(flagInitDevnet.IPs) > 1 {
		ipCount++
		if len(flagInitDevnet.IPs) < int(ipCount) {
			fatalf("not enough IPs")
		}
		return flagInitDevnet.IPs[ipCount-1]
	}

	if baseIP == nil {
		baseIP = net.ParseIP(flagInitDevnet.IPs[0])
		if baseIP == nil {
			fatalf("invalid IP: %q", flagInitDevnet.IPs[0])
		}
		if baseIP[15] == 0 {
			fatalf("invalid IP: base IP address must not end with .0")
		}
	}

	ip := make(net.IP, len(baseIP))
	copy(ip, baseIP)
	ip[15] += ipCount
	ipCount++
	return ip.String()
}

func writeDevnetDockerCompose(cmd *cobra.Command, netInit *accumulated.NetworkInit) {
	nodeCount := flagInitDevnet.NumValidators + flagInitDevnet.NumFollowers
	compose := new(dc.Config)
	compose.Version = "3"
	compose.Services = make([]dc.ServiceConfig, 0, 1+nodeCount*flagInitDevnet.NumBvns)
	compose.Volumes = make(map[string]dc.VolumeConfig, 1+nodeCount*flagInitDevnet.NumBvns)

	var i int
	for _, bvn := range netInit.Bvns {
		for range bvn.Nodes {
			i++
			var svc dc.ServiceConfig
			svc.Name = fmt.Sprintf("node-%d", i)
			svc.ContainerName = "devnet-" + svc.Name
			svc.Image = flagInitDevnet.DockerImage

			if flagInitDevnet.UseVolumes {
				svc.Volumes = []dc.ServiceVolumeConfig{
					{Type: "volume", Source: svc.Name, Target: "/node"},
				}
				compose.Volumes[svc.Name] = dc.VolumeConfig{}
			} else {
				svc.Volumes = []dc.ServiceVolumeConfig{
					{Type: "bind", Source: fmt.Sprintf("./node-%d", i), Target: "/node"},
				}
			}

			compose.Services = append(compose.Services, svc)
		}
	}

	var svc dc.ServiceConfig
	api := netInit.Bvns[0].Nodes[0].Advertize().Scheme("http").AccumulateAPI().String() + "/v2"
	svc.Name = "tools"
	svc.ContainerName = "devnet-init"
	svc.Image = flagInitDevnet.DockerImage
	svc.Environment = map[string]*string{"ACC_API": &api}
	extras := make(map[string]interface{})
	extras["profiles"] = [...]string{"init"}
	svc.Extras = extras

	svc.Command = dc.ShellCommand{"init", "devnet", "-w", "/nodes", "--docker"}
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		switch flag.Name {
		case "work-dir", "docker", "compose", "reset":
			return
		}

		s := fmt.Sprintf("--%s=%v", flag.Name, flag.Value)
		svc.Command = append(svc.Command, s)
	})

	if flagInitDevnet.UseVolumes {
		svc.Volumes = make([]dc.ServiceVolumeConfig, len(compose.Services))
		for i, node := range compose.Services {
			bits := strings.SplitN(node.Name, "-", 2)
			svc.Volumes[i] = dc.ServiceVolumeConfig{Type: "volume", Source: node.Name, Target: path.Join("/nodes", bits[0], "Node"+bits[1])}
		}
	} else {
		svc.Volumes = []dc.ServiceVolumeConfig{
			{Type: "bind", Source: ".", Target: "/nodes"},
		}
	}

	compose.Services = append(compose.Services, svc)

	apiPort := uint32(flagInitDevnet.BasePort) + uint32(cfg.PortOffsetAccumulateApi)
	dn0svc := compose.Services[0]
	dn0svc.Ports = []dc.ServicePortConfig{
		{Mode: "host", Protocol: "tcp", Target: apiPort, Published: apiPort},
	}

	check(os.MkdirAll(flagMain.WorkDir, 0755))
	f, err := os.Create(filepath.Join(flagMain.WorkDir, "docker-compose.yml"))
	check(err)
	defer f.Close()

	err = yaml.NewEncoder(f).Encode(compose)
	check(err)
}
