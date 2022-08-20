package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/kardianos/service"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
	service2 "github.com/tendermint/tendermint/libs/service"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

var cmdRun = &cobra.Command{
	Use:   "run",
	Short: "Run node",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runNode(cmd, args)
		printOutput(cmd, out, err)
	},
	Args: cobra.NoArgs,
}

var flagRun = struct {
	Node             int
	Truncate         bool
	CiStopAfter      time.Duration
	LogFile          string
	JsonLogFile      string
	EnableTimingLogs bool
	PprofListen      string
}{}

func init() {
	cmdMain.AddCommand(cmdRun)

	initRunFlags(cmdRun, false)
}

func initRunFlags(cmd *cobra.Command, forService bool) {
	cmd.ResetFlags()
	cmd.Flags().IntVarP(&flagRun.Node, "node", "n", -1, "Which node are we? [0, n)")
	cmd.PersistentFlags().BoolVar(&flagRun.Truncate, "truncate", false, "Truncate Badger if necessary")
	cmd.PersistentFlags().StringVar(&flagRun.LogFile, "log-file", "", "Write logs to a file as plain text")
	cmd.PersistentFlags().StringVar(&flagRun.JsonLogFile, "json-log-file", "", "Write logs to a file as JSON")
	cmd.PersistentFlags().BoolVar(&flagRun.EnableTimingLogs, "enable-timing-logs", false, "Enable core timing analysis logging")
	cmd.PersistentFlags().StringVar(&flagRun.PprofListen, "pprof", "", "Address to run net/http/pprof on")

	if !forService {
		cmd.Flags().DurationVar(&flagRun.CiStopAfter, "ci-stop-after", 0, "FOR CI ONLY - stop the node after some time")
		cmd.Flag("ci-stop-after").Hidden = true
	}

	cmd.PersistentPreRun = func(*cobra.Command, []string) {
		badger.TruncateBadger = flagRun.Truncate

		if flagRun.PprofListen != "" {
			go func() { check(http.ListenAndServe(flagRun.PprofListen, nil)) }()
		}
	}
}

func runNode(cmd *cobra.Command, _ []string) (string, error) {
	prog := NewProgram(cmd, singleNodeWorkDir, nil)

	svc, err := service.New(prog, serviceConfig)
	if err != nil {
		return "", err
	}

	logger, err := svc.Logger(nil)
	if err != nil {
		return "", err
	}

	if flagRun.CiStopAfter != 0 {
		go watchDog(prog, svc, flagRun.CiStopAfter)
	}
	color.HiGreen("------ starting a new node ------")

	err = svc.Run()
	if err != nil {
		//if it is already stopped, that is ok.
		if !errors.Is(err, service2.ErrAlreadyStopped) {
			_ = logger.Error(err)
			return "", err
		}
	}
	return "shutdown complete", nil
}

func watchDog(prog *Program, svc service.Service, duration time.Duration) {
	time.Sleep(duration)

	//this will cause tendermint to stop and exit cleanly.
	_ = prog.Stop(svc)

	//the following will stop the Run()
	interrupt(syscall.Getpid())
}

type logAnnotator func(io.Writer, string, bool) io.Writer

func newLogWriter(s service.Service) func(string, logAnnotator) (io.Writer, error) {
	// Each log file writer must be created once, otherwise different copies
	// will step on each other
	var logFile *rotateWriter
	if flagRun.LogFile != "" {
		logFile = &rotateWriter{file: flagRun.LogFile}
		check(logFile.Open())
	}

	var jsonLogFile *rotateWriter
	if flagRun.JsonLogFile != "" {
		jsonLogFile = &rotateWriter{file: flagRun.JsonLogFile}
		check(jsonLogFile.Open())
	}

	// Rotate logs on SIGHUP
	onHUP(func() {
		if logFile != nil {
			logFile.Rotate()
		}
		if jsonLogFile != nil {
			jsonLogFile.Rotate()
		}
	})

	return func(format string, annotate logAnnotator) (io.Writer, error) {
		var mainWriter io.Writer
		var err error
		if !service.Interactive() && s != nil {
			mainWriter, err = logging.NewServiceLogger(s, format)
			check(err)
		} else {
			mainWriter = os.Stderr
			if annotate != nil {
				mainWriter = annotate(mainWriter, format, true)
			}
			mainWriter, err = logging.NewConsoleWriterWith(mainWriter, format)
			check(err)
		}

		var writers multiWriter
		writers = append(writers, mainWriter)

		if logFile != nil {
			w := io.Writer(logFile)
			if annotate != nil {
				w = annotate(w, log.LogFormatText, false)
			}
			writers = append(writers, &zerolog.ConsoleWriter{
				Out:        w,
				NoColor:    true,
				TimeFormat: time.RFC3339,
				FormatLevel: func(i interface{}) string {
					if ll, ok := i.(string); ok {
						return strings.ToUpper(ll)
					}
					return "????"
				},
			})
		}

		if jsonLogFile != nil {
			w := io.Writer(jsonLogFile)
			if annotate != nil {
				w = annotate(w, log.LogFormatJSON, false)
			}
			writers = append(writers, w)
		}

		if len(writers) == 1 {
			return writers[0], nil
		}
		return writers, nil
	}
}

type multiWriter []io.Writer

func (w multiWriter) Write(b []byte) (int, error) {
	for _, w := range w {
		n, err := w.Write(b)
		if err != nil {
			return n, err
		}
		if n != len(b) {
			return n, io.ErrShortWrite
		}
	}
	return len(b), nil
}

func (w multiWriter) Close() error {
	var errs []error
	for _, w := range w {
		c, ok := w.(io.Closer)
		if !ok {
			continue
		}
		err := c.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return nil
	default:
		return errors.New(fmt.Sprint(errs))
	}
}

type rotateWriter struct {
	file string
	w    *os.File
	mu   sync.Mutex
}

func (w *rotateWriter) Open() error {
	if w.w != nil {
		return nil
	}

	var err error
	w.w, err = os.Create(w.file)
	return err
}

func (w *rotateWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	err := w.Open()
	if err != nil {
		return 0, err
	}

	return w.w.Write(b)
}

func (w *rotateWriter) Rotate() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.w == nil {
		return
	}

	err := w.w.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "rotate writer failed to close file: %v\n", err)
	}

	w.w = nil
}
