package simulator

import "fmt"

type TB interface {
	Name() string
	Log(...interface{})
	Fail()
	FailNow()
	Helper()
}

type tb struct {
	TB
}

func (t tb) Logf(format string, args ...interface{}) {
	t.Helper()
	t.Log(fmt.Sprintf(format, args...))
}

func (t tb) Errorf(format string, args ...interface{}) {
	t.Helper()
	t.Logf(format, args...)
	t.Fail()
}

func (t tb) Fatalf(format string, args ...interface{}) {
	t.Helper()
	t.Logf(format, args...)
	t.FailNow()
}
