package main

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
)

func init() {
	core.Warnln = func(message string) { warnln(message) }
	core.Infof = infof
	core.Infoln = infoln
	core.Warnf = warnf
	core.VerboseLogf = verboseLogf
	core.VerboseEnabled = func() bool { return currentVerboseState().enabled }
	core.VerboseWriter = func() any { return currentVerboseState().out }
	core.SpinnerAwareStdoutWrite = compose.SpinnerAwareStdoutWrite
	core.SpinnerAwareStderrWrite = compose.SpinnerAwareStderrWrite
}
