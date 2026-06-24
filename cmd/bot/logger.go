package main

import (
	"fmt"

	"go.temporal.io/sdk/log"
)

// noopLogger satisfies the Temporal log.Logger interface and discards everything.
type noopLogger struct{}

func (noopLogger) Debug(_ string, _ ...interface{}) {}
func (noopLogger) Info(_ string, _ ...interface{})  {}
func (noopLogger) Warn(_ string, _ ...interface{})  {}
func (noopLogger) Error(_ string, _ ...interface{}) {}

// stdLogger forwards to Go's standard log package (used when -v is set).
type stdLogger struct{}

func (stdLogger) Debug(msg string, kv ...interface{}) { logKV("DEBUG", msg, kv...) }
func (stdLogger) Info(msg string, kv ...interface{})  { logKV("INFO ", msg, kv...) }
func (stdLogger) Warn(msg string, kv ...interface{})  { logKV("WARN ", msg, kv...) }
func (stdLogger) Error(msg string, kv ...interface{}) { logKV("ERROR", msg, kv...) }

func logKV(level, msg string, kv ...interface{}) {
	// format: LEVEL msg key=value key=value ...
	s := level + " " + msg
	for i := 0; i+1 < len(kv); i += 2 {
		s += " " + sprint(kv[i]) + "=" + sprint(kv[i+1])
	}
	println(s) //nolint:forbidigo
}

func sprint(v interface{}) string {
	if v == nil {
		return "<nil>"
	}
	type stringer interface{ String() string }
	if s, ok := v.(stringer); ok {
		return s.String()
	}
	return fmt.Sprintf("%v", v)
}

func newTemporalLogger(verbose bool) log.Logger {
	if verbose {
		return stdLogger{}
	}
	return noopLogger{}
}
