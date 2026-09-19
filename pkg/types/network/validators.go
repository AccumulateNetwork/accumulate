// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package network

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/robfig/cron/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var CronFormat = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

func (g *GlobalValues) ChainID() *big.Int {
	return protocol.EthChainID(g.Network.NetworkName)
}

type globalValueMemos struct {
	bvns               []string
	threshold          map[string]uint64
	active             map[string]int
	majorBlockSchedule cron.Schedule
}

func (g *GlobalValues) memoizeValidators() {
	if g.memoize.active != nil {
		return
	}

	var bvns []string
	for _, p := range g.Network.Partitions {
		if p.Type == protocol.PartitionTypeBlockValidator {
			bvns = append(bvns, p.ID)
		}
	}

	active := make(map[string]int, len(g.Network.Partitions))
	for _, v := range g.Network.Validators {
		for _, p := range v.Partitions {
			if p.Active {
				active[strings.ToLower(p.ID)]++
			}
		}
	}

	threshold := make(map[string]uint64, len(g.Network.Partitions))
	for partition, active := range active {
		threshold[partition] = g.Globals.ValidatorAcceptThreshold.Threshold(active)
	}

	g.memoize.bvns = bvns
	g.memoize.active = active
	g.memoize.threshold = threshold
}

func (g *GlobalValues) BvnExecutorVersion() protocol.ExecutorVersion {
	if len(g.BvnExecutorVersions) == 0 {
		return 0
	}

	v := g.ExecutorVersion
	for _, b := range g.BvnExecutorVersions {
		if b.Version < v {
			v = b.Version
		}
	}
	return v
}

func (g *GlobalValues) BvnNames() []string {
	g.memoizeValidators()
	return g.memoize.bvns
}

func (g *GlobalValues) ValidatorThreshold(partition string) uint64 {
	g.memoizeValidators()
	v, ok := g.memoize.threshold[strings.ToLower(partition)]
	if !ok {
		return math.MaxUint64
	}
	return v
}

func (g *GlobalValues) MajorBlockSchedule() cron.Schedule {
	if g.memoize.majorBlockSchedule != nil {
		return g.memoize.majorBlockSchedule
	}
	s, err := CronFormat.Parse(g.Globals.MajorBlockSchedule)
	if err != nil {
		panic(fmt.Errorf("cannot parse major block schedule: %w", err))
	}
	g.memoize.majorBlockSchedule = s
	return s
}

// CommitteeMembership is what the network definition says about one key's
// standing in one partition's committee.
//
// Three values and not two, because "this node holds no definition yet" is a
// different fact from "this key is not a validator", and the two callers that
// ask need OPPOSITE defaults for it: the submit path must not refuse traffic
// on a startup race, and the anchor path must not sign on one. A boolean
// forces one of them to be wrong, which is how the same question came to have
// two implementations that disagreed (#4366, #4367).
type CommitteeMembership int

const (
	// CommitteeUnknown: no globals, or a definition with no validators at
	// all. Nothing is known about this key; the caller decides what to do
	// with not knowing, and says so where it decides.
	CommitteeUnknown CommitteeMembership = iota

	// CommitteeMember: the key is a validator active on the partition.
	CommitteeMember

	// CommitteeOutsider: the definition is known and this key is not an
	// active validator of the partition — a follower, a validator active
	// elsewhere, one removed on chain, or no key at all.
	CommitteeOutsider
)

// MembershipOf reports the standing of an ed25519 public key in a partition's
// committee, as these globals give it.
//
// Safe on a nil receiver: a node that has loaded nothing yet answers
// CommitteeUnknown rather than panicking, which is what an atomic load of the
// globals gives before the first WillChangeGlobals.
//
// The validators are walked rather than binary-searched. ValidatorByKey is a
// search over PublicKeyHash, so a definition that is unsorted or whose hashes
// are unset answers "not in the network", and a false outsider means refusing
// every submission or withholding every anchor. Twelve validators is a walk,
// and both the stored key and its hash are compared, so an entry carrying
// only one of them still matches.
func (g *GlobalValues) MembershipOf(key []byte, partition string) CommitteeMembership {
	if len(key) != ed25519.PublicKeySize {
		// No identity to be in a committee with (#4367).
		return CommitteeOutsider
	}
	if g == nil || g.Network == nil || len(g.Network.Validators) == 0 {
		return CommitteeUnknown
	}

	hash := sha256.Sum256(key)
	for _, v := range g.Network.Validators {
		if !bytes.Equal(v.PublicKey, key) && v.PublicKeyHash != hash {
			continue
		}
		if v.IsActiveOn(partition) {
			return CommitteeMember
		}
		return CommitteeOutsider
	}
	return CommitteeOutsider
}
