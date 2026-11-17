package core

// Architectural guards for the logging and CLI output design.
// See docs/dev/reference/conventions.md#logging-and-cli-output and
// docs/dev/explanation/logging-architecture.md.

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// TestNewCLILoggerWritesToStderrNotStdout verifies the architectural
// invariant that *slog.Logger (the operational event channel) does not
// contaminate stdout (the user-text channel). A regression here would
// break every integration test that asserts user text is present on
// stdout — slog-formatted lines would appear there instead.
func TestNewCLILoggerWritesToStderrNotStdout(t *testing.T) {
	stdout := captureStream(t, &os.Stdout, func() {
		logger := NewCLILogger(false)
		logger.Info("operational event", "key", "value")
		logger.Warn("degraded state", "key", "value")
	})
	if stdout != "" {
		t.Errorf("CLI slog logger wrote to stdout; must use stderr only.\nGot stdout: %q", stdout)
	}
}

// TestNewCLILoggerVerboseFlagControlsDebug verifies that Debug output is
// gated by the verbose flag. Not a channel rule, but it is the only
// knob operators have to control slog verbosity, and a regression would
// either flood stderr or lose trace entirely.
func TestNewCLILoggerVerboseFlagControlsDebug(t *testing.T) {
	stderr := captureStream(t, &os.Stderr, func() {
		logger := NewCLILogger(false)
		logger.Debug("trace event")
	})
	if strings.Contains(stderr, "trace event") {
		t.Errorf("Debug-level emitted at default verbosity; expected Info gate.\nGot: %q", stderr)
	}
	stderr = captureStream(t, &os.Stderr, func() {
		logger := NewCLILogger(true)
		logger.Debug("trace event")
	})
	if !strings.Contains(stderr, "trace event") {
		t.Errorf("Debug-level suppressed even with verbose=true.\nGot: %q", stderr)
	}
}

// TestPrintJSONOutputEmitsParseableJSON verifies that the JSON output
// path produces valid JSON on stdout with no surrounding text. A
// regression here would break scripted consumers who expect
// `ark-cli ... --json` to pipe into `jq`.
func TestPrintJSONOutputEmitsParseableJSON(t *testing.T) {
	stdout := captureStream(t, &os.Stdout, func() {
		if err := PrintJSONOutput(map[string]any{"name": "asa1", "port": 27015}); err != nil {
			t.Fatalf("PrintJSONOutput: %v", err)
		}
	})
	var parsed any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Errorf("stdout is not valid JSON: %v\nstdout:\n%s", err, stdout)
	}
	if strings.Contains(stdout, "name: asa1") {
		t.Errorf("stdout contains YAML-ish text alongside JSON; should be pure JSON.\nstdout:\n%s", stdout)
	}
}

// TestPrintJSONOutputEmitsEmptyCollection verifies that an empty slice
// marshals to `[]`, not `null`. List commands with no rows must still
// emit a valid JSON array.
func TestPrintJSONOutputEmitsEmptyCollection(t *testing.T) {
	stdout := captureStream(t, &os.Stdout, func() {
		if err := PrintJSONOutput([]ScheduleEntryJSON{}); err != nil {
			t.Fatalf("PrintJSONOutput: %v", err)
		}
	})
	got := strings.TrimSpace(stdout)
	if got != "[]" {
		t.Errorf("empty slice should marshal to []; got %q", got)
	}
}

// TestDefaultInfofWritesToStdout verifies that the default Infof
// implementation (used when app/ark-cli/utils.go init has not run, i.e.
// in unit tests) targets stdout. This matches what
// testutil.CaptureStdout expects, and regressions would silently break
// hundreds of Go unit test assertions.
func TestDefaultInfofWritesToStdout(t *testing.T) {
	stdout := captureStream(t, &os.Stdout, func() {
		Infof("user text: %s\n", "hello")
	})
	if !strings.Contains(stdout, "user text: hello") {
		t.Errorf("default Infof did not write to stdout.\nGot: %q", stdout)
	}
}

// TestDefaultWarnfWritesToStderr verifies the complement of the Infof
// invariant: default Warnf targets stderr.
func TestDefaultWarnfWritesToStderr(t *testing.T) {
	stderr := captureStream(t, &os.Stderr, func() {
		Warnf("warning: %s\n", "oops")
	})
	if !strings.Contains(stderr, "warning: oops") {
		t.Errorf("default Warnf did not write to stderr.\nGot: %q", stderr)
	}
}

// TestDefaultPromptfWritesToStderr verifies Promptf defaults to stderr.
// Prompts must go on stderr so a piped stdout consumer (e.g. `--format
// json | jq`) never sees stray prompt text; see conventions.md
// §"Logging and CLI output".
func TestDefaultPromptfWritesToStderr(t *testing.T) {
	stderr := captureStream(t, &os.Stderr, func() {
		Promptf("Continue? [y/N]: ")
	})
	if stderr != "Continue? [y/N]: " {
		t.Errorf("Promptf output mismatch.\nGot: %q\nWant: %q", stderr, "Continue? [y/N]: ")
	}
	// And not to stdout.
	stdout := captureStream(t, &os.Stdout, func() {
		Promptf("label: ")
	})
	if stdout != "" {
		t.Errorf("Promptf leaked to stdout: %q", stdout)
	}
}

// TestDefaultPromptfDoesNotAppendNewline verifies the prompt remains on
// the same line as the response. A trailing newline would push the
// user's cursor to a new line.
func TestDefaultPromptfDoesNotAppendNewline(t *testing.T) {
	stderr := captureStream(t, &os.Stderr, func() {
		Promptf("name")
	})
	if stderr != "name" {
		t.Errorf("Promptf added unexpected content: %q", stderr)
	}
}

// TestDefaultWarnlnWritesToStderr verifies that Warnln has a working
// default. Previously it was a nil function variable, so any test or
// non-CLI caller of Die would panic before exiting.
func TestDefaultWarnlnWritesToStderr(t *testing.T) {
	stderr := captureStream(t, &os.Stderr, func() {
		Warnln("something went wrong")
	})
	if !strings.Contains(stderr, "something went wrong") {
		t.Errorf("Warnln did not write to stderr.\nGot: %q", stderr)
	}
}

// captureStream redirects the given *os.File (os.Stdout or os.Stderr)
// during fn and returns everything written to it. Independent of
// testutil.CaptureStdout/Stderr because this file is in the core
// package and must not import testutil (import cycle).
//
// When verifying that a slog.Logger obtained via slog.Default() or any
// handler whose destination is resolved at call time writes to the
// expected stream, the stream pointer must be replaced (not a copy),
// hence the **os.File parameter.
func captureStream(t *testing.T, stream **os.File, fn func()) string {
	t.Helper()
	old := *stream
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	*stream = w
	defer func() { *stream = old }()

	done := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()

	fn()
	_ = w.Close()
	var buf bytes.Buffer
	buf.Write(<-done)
	return buf.String()
}
