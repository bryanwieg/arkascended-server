package core

// Tests: CLI argument parsing: interspersed flags, double-dash separator, extra-args rejection

import (
	"strings"
	"testing"
)

// TestParseCommandArgsInterspersedFlags verifies command-level flags can appear around positionals.
// - Expect: positionals and flags are parsed consistently regardless of ordering.
func TestParseCommandArgsInterspersedFlags(t *testing.T) {
	fs := NewCommandFlagSet("test", nil, "")
	composeFile := fs.String("compose-file", "", "")
	autoStop := fs.Bool("auto-stop", false, "")

	parsed, err := ParseCommandArgs(fs, []string{
		"asa1",
		"--compose-file", "docker-compose.dev.yaml",
		"latest",
		"--auto-stop",
	}, CommandArgSpec{
		MinPositionals: 2,
		MaxPositionals: 2,
		MissingError:   "missing",
	})
	if err != nil {
		t.Fatalf("ParseCommandArgs: %v", err)
	}
	if len(parsed.Positionals) != 2 {
		t.Fatalf("unexpected positional count: %d", len(parsed.Positionals))
	}
	if parsed.Positionals[0] != "asa1" || parsed.Positionals[1] != "latest" {
		t.Fatalf("unexpected positionals: %#v", parsed.Positionals)
	}
	if *composeFile != "docker-compose.dev.yaml" {
		t.Fatalf("unexpected compose-file: %s", *composeFile)
	}
	if !*autoStop {
		t.Fatal("expected bool flag to parse")
	}
}

// TestParseCommandArgsSeparatorPreservesDashedPositionals ensures `--` terminates flag parsing.
// - Expect: tokens after `--` are treated as positionals, even if they begin with '-'.
func TestParseCommandArgsSeparatorPreservesDashedPositionals(t *testing.T) {
	fs := NewCommandFlagSet("test", nil, "")

	parsed, err := ParseCommandArgs(fs, []string{"--", "--literal-name"}, CommandArgSpec{
		MinPositionals: 1,
		MaxPositionals: 1,
		MissingError:   "missing",
	})
	if err != nil {
		t.Fatalf("ParseCommandArgs: %v", err)
	}
	if len(parsed.Positionals) != 1 || parsed.Positionals[0] != "--literal-name" {
		t.Fatalf("unexpected positionals: %#v", parsed.Positionals)
	}
}

// TestParseCommandArgsRejectsExtras ensures shared helpers preserve standard extra-arg errors.
// - Expect: extra positionals return the canonical unexpected-extra-arguments error.
func TestParseCommandArgsRejectsExtras(t *testing.T) {
	fs := NewCommandFlagSet("test", nil, "")

	_, err := ParseCommandArgs(fs, []string{"one", "two"}, CommandArgSpec{
		MinPositionals: 0,
		MaxPositionals: 1,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unexpected extra arguments") {
		t.Fatalf("unexpected error: %v", err)
	}
}
