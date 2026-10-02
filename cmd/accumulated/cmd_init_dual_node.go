// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

var cmdInitDualNode = &cobra.Command{
	Use:   "dual <peer url>",
	Short: "Initialize a node that runs the Directory and a BVN, from a peer that runs both (its base port, where its API is)",
	Run:   initDualNode,
	Args:  cobra.ExactArgs(1),
}

// initDualNode is `accumulated init dual tcp://ip:port`: it writes the same
// layout as `init node`, for a node that runs the Directory and the peer's BVN.
func initDualNode(cmd *cobra.Command, args []string) {
	if !cmd.Flag("work-dir").Changed {
		fatalf("--work-dir flag is required")
	}
	if flagInitDualNode.SeedProxy != "" {
		fatalf("Starting from a seed proxy is not supported by init dual")
	}

	public := flagInitDualNode.PublicIP
	if public == "" {
		var err error
		public, err = resolvePublicIp()
		checkf(err, "cannot resolve public ip address")
	}

	flagInitNode.SkipVersionCheck = flagInitDualNode.SkipVersionCheck
	j, err := joinFromPeer(cmd.Context(), args[0], nil, flagInitDualNode.DnGenesis, flagInitDualNode.BvnGenesis)
	checkf(err, "failed to configure node from peer")
	if !j.dn || j.bvn == "" {
		fatalf("%s does not run both the Directory and a BVN", args[0])
	}

	if flagInit.Reset {
		resetNodeDir(flagMain.WorkDir)
	}

	err = writeNodeConfig(flagMain.WorkDir, j, nodeInitOptions{
		Listen:       flagInitDualNode.ListenIP,
		Public:       public,
		Follower:     flagInitDualNode.Follower,
		NoPrometheus: flagInitDualNode.NoPrometheus,
	})
	checkf(err, "write node files")
}

func resolvePublicIp() (string, error) {
	req, err := http.Get("http://ip-api.com/json/")
	if err != nil {
		return "", err
	}
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "", err
	}

	ip := struct {
		Query string
	}{}
	err = json.Unmarshal(body, &ip)
	if err != nil {
		return "", err
	}
	return ip.Query, nil
}
