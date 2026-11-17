package core

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// CommandArgSpec specifies the positional-argument constraints for a command.
type CommandArgSpec struct {
	MinPositionals int
	MaxPositionals int
	MissingError   string
	ExtraError     string
}

// ParsedCommandArgs holds the result of parsing command arguments.
type ParsedCommandArgs struct {
	Positionals []string
}

type boolFlagValue interface {
	IsBoolFlag() bool
}

// NewCommandFlagSet creates a flag.FlagSet for a CLI command with standard
// usage formatting. preferred lists the usage lines; summary is an optional
// one-liner printed after them.
func NewCommandFlagSet(name string, preferred []string, summary string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	ConfigureFlagSetUsage(fs, preferred, summary)
	return fs
}

// ConfigureFlagSetUsage sets a custom Usage function on fs that prints
// preferred usage lines, an optional summary, and then the flag defaults.
func ConfigureFlagSetUsage(fs *flag.FlagSet, preferred []string, summary string) {
	fs.Usage = func() {
		out := fs.Output()
		for _, line := range preferred {
			fmt.Fprintf(out, "Usage: ark-cli %s\n", line)
		}
		if summary != "" {
			fmt.Fprintf(out, "%s\n", summary)
		}
		fmt.Fprintln(out, "Options:")
		fs.PrintDefaults()
	}
}

// ParseCommandArgs separates flag arguments from positional arguments and
// validates against spec.
func ParseCommandArgs(fs *flag.FlagSet, args []string, spec CommandArgSpec) (ParsedCommandArgs, error) {
	flagArgs, positionalArgs := SplitInterspersedFlagArgs(fs, args)
	if err := fs.Parse(flagArgs); err != nil {
		return ParsedCommandArgs{}, err
	}
	if len(positionalArgs) < spec.MinPositionals {
		if spec.MissingError != "" {
			return ParsedCommandArgs{}, errors.New(spec.MissingError)
		}
		return ParsedCommandArgs{}, errors.New("Error: missing required arguments.")
	}
	if spec.MaxPositionals >= 0 && len(positionalArgs) > spec.MaxPositionals {
		if spec.ExtraError != "" {
			return ParsedCommandArgs{}, errors.New(spec.ExtraError)
		}
		return ParsedCommandArgs{}, errors.New("Error: unexpected extra arguments.")
	}
	return ParsedCommandArgs{Positionals: positionalArgs}, nil
}

// SplitInterspersedFlagArgs separates flag arguments from positional arguments,
// respecting "--" as a terminator.
func SplitInterspersedFlagArgs(fs *flag.FlagSet, args []string) ([]string, []string) {
	flagArgs := make([]string, 0, len(args))
	positionalArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionalArgs = append(positionalArgs, args[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			positionalArgs = append(positionalArgs, arg)
			continue
		}

		flagArgs = append(flagArgs, arg)
		if strings.Contains(arg, "=") {
			continue
		}

		name := strings.TrimLeft(arg, "-")
		if name == "" {
			continue
		}
		flagDef := fs.Lookup(name)
		if flagDef == nil || isBoolFlagValue(flagDef) {
			continue
		}
		if i+1 >= len(args) {
			continue
		}
		flagArgs = append(flagArgs, args[i+1])
		i++
	}
	return flagArgs, positionalArgs
}

func isBoolFlagValue(f *flag.Flag) bool {
	bf, ok := f.Value.(boolFlagValue)
	return ok && bf.IsBoolFlag()
}

// ParseNoPositionalArgs parses flags and rejects any positional arguments.
func ParseNoPositionalArgs(fs *flag.FlagSet, args []string) error {
	_, err := ParseCommandArgs(fs, args, CommandArgSpec{MinPositionals: 0, MaxPositionals: 0})
	return err
}
