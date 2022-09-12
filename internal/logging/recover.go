package logging

import (
	stdlog "log"
	"runtime/debug"

	"github.com/tendermint/tendermint/libs/log"
)

func Recover(logger log.Logger, message string, values ...interface{}) {
	defer func() {
		_ = recover()
		println("Panicked while recovering from a panic") //nolint:noprint
	}()

	err := recover()
	if err == nil {
		return
	}
	if logger == nil {
		stdlog.Printf("Panicked without a logger\n%v\n%s\n", err, debug.Stack())
		return
	}

	values = append(values, "error", err, "stack", debug.Stack())
	logger.Error("message", values...)
}
