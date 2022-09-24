package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var fixCmd = &cobra.Command{
	Use:   "fix <partition ID> <snapshot>",
	Short: "Fix the header of a snapshot affected by AC-3210",
	Args:  cobra.ExactArgs(2),
	Run:   fixSnapshot,
}

func init() {
	cmd.AddCommand(fixCmd)
}

func fixSnapshot(_ *cobra.Command, args []string) {
	partition := args[0]
	filename := args[1]
	if strings.EqualFold(partition, "dn") {
		partition = protocol.Directory
	}

	// Temp dir
	tmpdir, err := os.MkdirTemp("", "accumulate-fix-snapshot-*")
	check(err)
	defer func() { os.RemoveAll(tmpdir) }()

	// Get height and hash
	height, rootHash := restoreSnapshot(filename, filepath.Join(tmpdir, "badger.db"), partition)

	// Copy to temp file
	tmpfile := filepath.Join(tmpdir, "snapshot.bpt")
	copyFile(tmpfile, filename)

	// Fix the header
	rewriteHeader(filename, tmpfile, height, rootHash)
}

// restoreSnapshot restores a snapshot and extracts the height and root hash.
func restoreSnapshot(filename, badgerPath, partitionID string) (uint64, []byte) {
	// Logger
	writer, err := logging.NewConsoleWriter("plain")
	check(err)
	level, writer, err := logging.ParseLogLevel(config.DefaultLogLevels, writer)
	check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(writer), level, false)
	check(err)

	// Open snapshot
	f, err := os.Open(filename)
	checkf(err, "open snapshot %s", filename)
	defer f.Close()

	// Open database
	db, err := database.OpenBadger(badgerPath, logger)
	check(err)

	// Restore
	err = snapshot.Restore(db, f, logger)
	check(err)

	// Get height and hash
	partition := protocol.PartitionUrl(partitionID)
	batch := db.Begin(false)
	defer batch.Discard()
	rootHash := batch.BptRoot()

	var ledger *protocol.SystemLedger
	err = batch.Account(partition.JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	check(err)

	return ledger.Index, rootHash
}

func copyFile(dst, src string) {
	fdst, err := os.Create(dst)
	check(err)
	defer fdst.Close()

	fsrc, err := os.Open(src)
	check(err)
	defer fsrc.Close()

	_, err = io.Copy(fdst, fsrc)
	check(err)
}

func rewriteHeader(dst, src string, height uint64, rootHash []byte) {
	fdst, err := os.Create(dst)
	check(err)
	defer fdst.Close()
	w := snapshot.NewWriter(fdst)

	fsrc, err := os.Open(src)
	check(err)
	defer fsrc.Close()
	r := snapshot.NewReader(fsrc)

	for {
		s, err := r.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				check(err)
			}
			break
		}

		sr, err := s.Open()
		check(err)

		// Copy the section
		if s.Type() != snapshot.SectionTypeHeader {
			sw, err := w.Open(s.Type())
			check(err)
			_, err = io.Copy(sw, sr)
			check(err)
			err = sw.Close()
			check(err)
			continue
		}

		header := new(snapshot.Header)
		_, err = header.ReadFrom(sr)
		check(err)

		header.Height = height
		header.RootHash = *(*[32]byte)(rootHash)

		sw, err := w.Open(s.Type())
		check(err)
		_, err = header.WriteTo(sw)
		check(err)
		err = sw.Close()
		check(err)
	}
}
