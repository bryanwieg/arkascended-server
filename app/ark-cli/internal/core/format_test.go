package core

import (
	"os"
	"strings"
	"testing"
)

func TestPrintDetailKVAligns(t *testing.T) {
	stdout := captureStream(t, &os.Stdout, func() {
		PrintDetailKV([][2]string{
			{"name", "asa1"},
			{"status", "running"},
			{"port", "27015"},
			{"session", "MyServer"},
		})
	})
	// Longest key is "session" (7 chars); values start at column 10
	// (0-indexed) = column 11 (1-indexed).
	want := "name:     asa1\n" +
		"status:   running\n" +
		"port:     27015\n" +
		"session:  MyServer\n"
	if stdout != want {
		t.Errorf("output mismatch;\ngot:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestPrintDetailKVMatchesShowInstanceFormat(t *testing.T) {
	// Golden-file guard: the block produced by PrintDetailKV with the
	// exact keys used by `show instance` today must match the
	// handwritten output byte-for-byte. This lets a future remediation
	// PR migrate the handler call site without changing any output,
	// and without breaking integration tests that assert on specific
	// whitespace (e.g. "port:     27015").
	stdout := captureStream(t, &os.Stdout, func() {
		PrintDetailKV([][2]string{
			{"name", "asa1"},
			{"status", "running"},
			{"image", "ark-server:latest"},
			{"version", "-"},
			{"map", "TheIsland_WP"},
			{"session", "MyServer"},
			{"port", "27015"},
		})
	})
	// Match the handwritten alignment at app/ark-cli/internal/show/instance.go:145–151.
	want := "name:     asa1\n" +
		"status:   running\n" +
		"image:    ark-server:latest\n" +
		"version:  -\n" +
		"map:      TheIsland_WP\n" +
		"session:  MyServer\n" +
		"port:     27015\n"
	if stdout != want {
		t.Errorf("output mismatch vs handwritten show instance format;\ngot:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestPrintDetailKVEmpty(t *testing.T) {
	stdout := captureStream(t, &os.Stdout, func() {
		PrintDetailKV(nil)
		PrintDetailKV([][2]string{})
	})
	if stdout != "" {
		t.Errorf("empty input produced output: %q", stdout)
	}
}

func TestPrintDetailKVSingleRow(t *testing.T) {
	stdout := captureStream(t, &os.Stdout, func() {
		PrintDetailKV([][2]string{{"key", "value"}})
	})
	// Single row: spaces = 3+2-3 = 2.
	if stdout != "key:  value\n" {
		t.Errorf("single row mismatch; got %q", stdout)
	}
}

func TestPrintPipeTableHeaderPlusRows(t *testing.T) {
	stdout := captureStream(t, &os.Stdout, func() {
		PrintPipeTable([][]string{
			{"instance", "action"},
			{"asa1", "update"},
			{"asa-server-2", "skip"},
		})
	})
	// Widest column 0 = "asa-server-2" (12). Widest column 1 = "update" (6).
	want := "instance     | action\n" +
		"-------------|-------\n" +
		"asa1         | update\n" +
		"asa-server-2 | skip  \n"
	if stdout != want {
		t.Errorf("output mismatch;\ngot:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestPrintPipeTableEmptyAndHeaderOnly(t *testing.T) {
	stdout := captureStream(t, &os.Stdout, func() {
		PrintPipeTable(nil)
		PrintPipeTable([][]string{{"just", "a", "header"}})
	})
	if stdout != "" {
		t.Errorf("expected empty output for nil and header-only input; got %q", stdout)
	}
}

// TestPrintPipeTableHandlesAnsiEscapeCodesInCells verifies VisualLen is
// used for width computation so that ANSI-coloured cells don't throw
// off alignment.
func TestPrintPipeTableHandlesAnsiEscapeCodesInCells(t *testing.T) {
	stdout := captureStream(t, &os.Stdout, func() {
		PrintPipeTable([][]string{
			{"name", "status"},
			{"asa1", "\x1b[31mdown\x1b[0m"},
			{"asa2", "running"},
		})
	})
	// Visual width of "\x1b[31mdown\x1b[0m" is 4; "running" is 7.
	// The literal output still contains the escape codes; alignment
	// should be computed as if they weren't there.
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines; got %d\n%s", len(lines), stdout)
	}
	// Line 2 (colored): visible content "asa1 | <red>down<reset>" — after
	// the color code, padding brings visible width to 7.
	if !strings.Contains(lines[2], "\x1b[31mdown\x1b[0m") {
		t.Errorf("expected ANSI codes preserved in cell; got %q", lines[2])
	}
}
