package main

// Tests: global option parsing: --verbose flag, logfile variants, unknown-flag rejection, help flag detection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGlobalOptionsVerboseFlagOnly(t *testing.T) {
	opts, remaining, err := parseGlobalOptions([]string{"-v", "create", "instance", "asa1"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if !opts.verbose {
		t.Fatal("expected verbose=true")
	}
	if opts.logFile != "" {
		t.Fatalf("expected empty logfile, got %q", opts.logFile)
	}
	if len(remaining) != 3 || remaining[0] != "create" || remaining[1] != "instance" || remaining[2] != "asa1" {
		t.Fatalf("unexpected remaining args: %#v", remaining)
	}
}

func TestParseGlobalOptionsVerboseWithLogfile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "cli.log")
	opts, remaining, err := parseGlobalOptions([]string{"-v", logPath, "restore", "asa1", "latest"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if !opts.verbose {
		t.Fatal("expected verbose=true")
	}
	if opts.logFile != logPath {
		t.Fatalf("expected logfile %s, got %q", logPath, opts.logFile)
	}
	if len(remaining) != 3 || remaining[0] != "restore" {
		t.Fatalf("unexpected remaining args: %#v", remaining)
	}
}

func TestParseGlobalOptionsVerboseEqualsLogfile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "cli.log")
	opts, remaining, err := parseGlobalOptions([]string{"--verbose=" + logPath, "start", "asa1"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if !opts.verbose {
		t.Fatal("expected verbose=true")
	}
	if opts.logFile != logPath {
		t.Fatalf("expected logfile %s, got %q", logPath, opts.logFile)
	}
	if len(remaining) != 2 || remaining[0] != "start" || remaining[1] != "asa1" {
		t.Fatalf("unexpected remaining args: %#v", remaining)
	}
}

func TestParseGlobalOptionsUnknownFlag(t *testing.T) {
	_, _, err := parseGlobalOptions([]string{"--bad", "backup", "asa1"})
	if err == nil {
		t.Fatal("expected unknown option error")
	}
}

func TestParseGlobalOptionsHelpShortAndLong(t *testing.T) {
	tests := [][]string{
		{"-h"},
		{"--help"},
	}
	for _, args := range tests {
		opts, remaining, err := parseGlobalOptions(args)
		if err != nil {
			t.Fatalf("parseGlobalOptions(%v): %v", args, err)
		}
		if !opts.help {
			t.Fatalf("expected help=true for args %v", args)
		}
		if len(remaining) != 0 {
			t.Fatalf("expected no remaining args for %v, got %#v", args, remaining)
		}
	}
}

func TestParseGlobalOptionsVerboseFlagOnlyWithCheckVerb(t *testing.T) {
	opts, remaining, err := parseGlobalOptions([]string{"-v", "check", "reachability", "port", "27015"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if !opts.verbose {
		t.Fatal("expected verbose=true")
	}
	if opts.logFile != "" {
		t.Fatalf("expected empty logfile, got %q", opts.logFile)
	}
	if len(remaining) != 4 || remaining[0] != "check" || remaining[1] != "reachability" || remaining[2] != "port" {
		t.Fatalf("unexpected remaining args: %#v", remaining)
	}
}

func TestConfigureVerboseMirrorsCLIOutputToFile(t *testing.T) {
	t.Cleanup(func() {
		closeVerbose()
		_ = configureVerbose(globalOptions{})
	})
	closeVerbose()
	if err := configureVerbose(globalOptions{}); err != nil {
		t.Fatalf("reset configureVerbose: %v", err)
	}

	logPath := filepath.Join(t.TempDir(), "cli.log")
	if err := configureVerbose(globalOptions{verbose: true, logFile: logPath}); err != nil {
		t.Fatalf("configureVerbose: %v", err)
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			infof("hello verbose\n")
			warnf("warning verbose\n")
		})
	})
	closeVerbose()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read verbose log: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "[cli-stdout] hello verbose") {
		t.Fatalf("verbose log missing stdout mirror:\n%s", text)
	}
	if !strings.Contains(text, "[cli-stderr] warning verbose") {
		t.Fatalf("verbose log missing stderr mirror:\n%s", text)
	}
}
