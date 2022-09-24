package main

import (
	"encoding/csv"
	"io"
	"io/fs"
	"os"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdAdd = &cobra.Command{
	Use:   "add",
	Short: "Add records to a pre-genesis snapshot",
}

var cmdAddGovernance = &cobra.Command{
	Use:   "governance [snapshot] [CSVs]",
	Short: "Add governance ADIs to a pre-genesis snapshot",
	Args:  cobra.MinimumNArgs(2),
	Run:   addGovernance,
}

var cmdAddReserved = &cobra.Command{
	Use:   "reserved [snapshot] [CSVs]",
	Short: "Add reserved ADIs to a pre-genesis snapshot",
	Args:  cobra.MinimumNArgs(2),
	Run:   addReserved,
}

func init() {
	cmd.AddCommand(cmdAdd)
	cmdAdd.AddCommand(cmdAddGovernance, cmdAddReserved)

	cmdAddGovernance.Flags().StringVarP(&flags.LogLevel, "log-level", "l", "info", "Set the logging level")
	cmdAddGovernance.Flags().IntVar(&flags.UrlCol, "url-column", 1, "The column the URL is in (1-based index)")

	cmdAddReserved.Flags().StringVarP(&flags.LogLevel, "log-level", "l", "info", "Set the logging level")
	cmdAddReserved.Flags().IntVar(&flags.UrlCol, "url-column", 1, "The column the URL is in (1-based index)")
	cmdAddReserved.Flags().IntVar(&flags.OwnerCol, "owner-column", 2, "The column the owner is in (1-based index)")
}

func addGovernance(_ *cobra.Command, args []string) {
	operators := protocol.DnUrl().JoinPath(protocol.Operators)
	addToSnapshot(args[0], args[1:], func(file string, row int, b *database.Batch, u *url.URL, _ []string, logger log.Logger) {
		if !isValidIdentity(file, row, b, u, logger) {
			return
		}

		logger.Debug("Create governance ADI", "url", u)

		identity := new(protocol.ADI)
		book := new(protocol.KeyBook)
		page := new(protocol.KeyPage)
		identity.Url = u
		book.Url = u.JoinPath("book")
		page.Url = u.JoinPath("book", "1")

		identity.AddAuthority(book.Url)
		book.PageCount = 1
		book.BookType = protocol.BookTypeNormal
		book.AddAuthority(book.Url)
		page.AcceptThreshold = 1
		page.Version = 1
		page.AddKeySpec(&protocol.KeySpec{Delegate: operators})

		check(b.Account(identity.Url).Main().Put(identity))
		check(b.Account(book.Url).Main().Put(book))
		check(b.Account(page.Url).Main().Put(page))
	})
}

func addReserved(_ *cobra.Command, args []string) {
	addToSnapshot(args[0], args[1:], func(file string, row int, b *database.Batch, u *url.URL, record []string, logger log.Logger) {
		if !isValidIdentity(file, row, b, u, logger) {
			return
		}

		if flags.OwnerCol >= len(record) {
			logger.Info("Skipping row: missing owner URL", "row", row, "length", len(record))
			return
		}
		owner, err := url.Parse(record[flags.OwnerCol])
		if err != nil {
			logger.Error("Skipping row: invalid owner URL", "row", row, "value", record[flags.OwnerCol], "error", err)
			return
		}
		err = b.Account(owner).Main().GetAs(new(*protocol.KeyBook))
		if err != nil {
			logger.Error("Invalid record: invalid owner", "row", row, "url", owner, "error", err)
			return
		}

		logger.Debug("Create reserved ADI", "url", u, "owner", owner)

		identity := new(protocol.ADI)
		identity.Url = u
		identity.AddAuthority(owner)
		check(b.Account(identity.Url).Main().Put(identity))
	})
}

func addToSnapshot(filename string, files []string, process func(string, int, *database.Batch, *url.URL, []string, log.Logger)) {
	if flags.UrlCol <= 0 {
		flags.UrlCol = 0
	} else {
		flags.UrlCol--
	}

	if flags.OwnerCol <= 0 {
		flags.OwnerCol = 0
	} else {
		flags.OwnerCol--
	}

	logWriter, err := logging.NewConsoleWriter("plain")
	check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(logWriter), flags.LogLevel, false)
	check(err)

	dbdir, err := os.MkdirTemp("", "badger-*.db")
	check(err)
	defer func() { _ = os.RemoveAll(dbdir) }()

	db, err := database.OpenBadger(dbdir, logging.NullLogger{})
	checkf(err, "output database")
	defer db.Close()

	f, err := os.Open(filename)
	if err == nil {
		check(snapshot.Restore(db, f, logger))
		check(f.Close())
	} else if errors.Is(err, fs.ErrNotExist) {
		// Ok
	} else {
		checkf(err, "open snapshot")
	}

	for _, file := range files {
		batch := db.Begin(true)
		defer batch.Discard()

		f, err := os.Open(file)
		checkf(err, "open CSV")
		defer f.Close()

		rd := csv.NewReader(f)
		var row int
		for {
			row++
			record, err := rd.Read()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				checkf(err, "read %s record", file)
			}
			if flags.UrlCol >= len(record) {
				logger.Info("Skipping row: missing URL", "row", row, "length", len(record))
				continue
			}

			u, err := url.Parse(record[flags.UrlCol])
			if err != nil {
				logger.Error("Skipping row: invalid URL", "row", row, "value", record[flags.UrlCol], "error", err)
				continue
			}

			process(file, row, batch, u, record, logger)
		}

		check(batch.Commit())
	}

	// Create a snapshot
	f, err = os.Create(filename)
	checkf(err, "write snapshot")
	defer f.Close()
	check(db.View(func(batch *database.Batch) error {
		_, err := snapshot.Collect(batch, new(snapshot.Header), f, func(account *database.Account) (bool, error) { return true, nil })
		return err
	}))
}

func isValidIdentity(file string, row int, b *database.Batch, u *url.URL, logger log.Logger) bool {
	err := protocol.IsValidAdiUrl(u, false)
	if err != nil {
		logger.Info("Invalid ADI URL", "file", file, "row", row, "url", u, "error", err)
		return false
	}
	if !u.IsRootIdentity() {
		logger.Info("ADI URL is not a root identity", "file", file, "row", row, "url", u)
		return false
	}

	_, err = b.Account(u).Main().Get()
	switch {
	case err == nil:
		logger.Info("Skipping record: already exists", "file", file, "row", row, "url", u)
		return false
	case !errors.Is(err, errors.StatusNotFound):
		check(err)
	}

	return true
}
