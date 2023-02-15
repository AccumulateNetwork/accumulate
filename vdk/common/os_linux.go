package vdk

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

var ErrNotRoot = errors.New("cannot use 1Password: accumulate and the directory it is in must be owned by root")

func isOnePassWarn(err error) bool {
	return errors.Is(err, ErrNotRoot)
}

func canExecOnePass() error {
	// Verify the executable is owned by root
	this, err := os.Executable()
	if err != nil {
		return err
	}
	stat, err := os.Stat(this)
	if err != nil {
		return err
	}
	s := stat.Sys().(*syscall.Stat_t)
	if s.Uid != 0 {
		return ErrNotRoot
	}

	// Verify the executable's directory is owned by root
	dir := filepath.Dir(this)
	stat, err = os.Stat(dir)
	if err != nil {
		return err
	}
	s = stat.Sys().(*syscall.Stat_t)
	if s.Uid != 0 {
		return ErrNotRoot
	}
	return nil
}
