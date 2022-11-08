// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	stdurl "net/url"
	"os"
	"regexp"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/rpc/client/http"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var consensusCmd = &cobra.Command{
	Use:   "consensus",
	Short: "Get the status of consensus",
	Args:  cobra.MinimumNArgs(1),
	Run:   consensus,
}

func init() {
	cmd.AddCommand(consensusCmd)
}

func consensus(_ *cobra.Command, args []string) {
	netu, err := stdurl.Parse(args[0])
	checkf(err, "network url")
	netp, err := strconv.ParseUint(netu.Port(), 10, 64)
	checkf(err, "network url port")
	addr := fmt.Sprintf("http://%s:%d", netu.Hostname(), netp+uint64(config.PortOffsetTendermintRpc)+config.PortOffsetDirectory)
	netc, err := http.New(addr, addr+"/websocket")
	checkf(err, "rpc client")
	_, nodes := walkNetwork(args)

	r, err := netc.ConsensusState(context.Background())
	checkf(err, "query consensus state")

	var rs RoundState
	check(json.Unmarshal(r.RoundState, &rs))

	fmt.Printf("For %s\n", rs.HRS)

	prevotes := readVotes(rs.HeightVoteSet[0].Prevotes)
	precommits := readVotes(rs.HeightVoteSet[0].Precommits)

	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 1, ' ', 0)
	defer tw.Flush()

	fmt.Fprintf(tw, "Validator\tAddress\tPrevote\tPrecommit\n")
	for _, n := range nodes {
		if n.Info == nil || !n.Info.IsActiveOn(protocol.Directory) {
			continue
		}

		var addr string
		if n.Hostname != "" {
			addr = fmt.Sprintf("%s:%d", n.Hostname, n.BasePort)
		}

		var prevote string
		if prevotes[*(*[6]byte)(n.ValidatorID[:6])] {
			prevote = "✔"
		}

		var precommit string
		if precommits[*(*[6]byte)(n.ValidatorID[:6])] {
			precommit = "✔"
		}

		fmt.Fprintf(tw, "%x\t%s\t%s\t%s\n", sliceOrNil(n.ValidatorID), addr, prevote, precommit)
	}
}

var reVote = regexp.MustCompile(`^(?i)Vote\{\d+:([\da-f]+) `)

func readVotes(votes []string) map[[6]byte]bool {
	v := map[[6]byte]bool{}
	for _, x := range votes {
		m := reVote.FindStringSubmatch(x)
		if len(m) < 2 {
			continue
		}
		key, err := hex.DecodeString(m[1])
		checkf(err, "decode %s", x)
		v[*(*[6]byte)(key)] = true
	}
	return v
}

type RoundState struct {
	HRS           string          `json:"height/round/step"`
	HeightVoteSet []HeightVoteSet `json:"height_vote_set"`
}

type HeightVoteSet struct {
	Round      int
	Prevotes   []string
	Precommits []string
}
