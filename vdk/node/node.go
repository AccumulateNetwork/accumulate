// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package node

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/vdk/logger"
)

// NewNode specify working directory of configuration, a log writer callback,
// and nodeIndex (usually zero unless running a devnet with more than one node on the same BVN)
func NewNode(workDir string, logWriter logger.LogWriter, nodeIndex int) (*Daemon, error) {
	node, err := accumulated.Load(workDir, func(c *config.Config) (io.Writer, error) {
		la := func(w io.Writer, format string, color bool) io.Writer {
			config := logger.NodeWriterConfig{
				Format:          logger.NodeLogFormat(format),
				PartitionName:   c.Accumulate.PartitionId,
				NodeIndex:       nodeIndex,
				NodeName:        "node",
				NodeNamePadding: 0,
				Colorize:        color}
			return logger.NewNodeWriter(w, config)
		}
		return logWriter(c.LogFormat, la)
	})

	if err != nil {
		return nil, err
	}
	return node, nil
}
