package main

import (
	"errors"
	"fmt"

	"ark-cli/internal/cmd"
)

func parseCommandRoute(args []string) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, errors.New("Error: command is required.")
	}

	verb := args[0]
	spec, ok := cmd.CommandRegistryParseSpec(verb)
	if !ok {
		return "", nil, fmt.Errorf("Error: unknown command verb: %s", verb)
	}
	return parseRegistryVerbObjectRoute(args, spec.MissingErr, spec.WrongFmt)
}

func parseRegistryVerbObjectRoute(args []string, missingErr, wrongFmt string) (string, []string, error) {
	if len(args) < 2 {
		return "", nil, errors.New(missingErr)
	}
	entry, ok := cmd.CommandRegistryLookup(args[0], args[1])
	if !ok {
		return "", nil, fmt.Errorf(wrongFmt, args[1])
	}
	return entry.Route, args[2:], nil
}

func usage() {
	warnln("Usage: ark-cli [--version] [-v[=LOGFILE]|--verbose[=LOGFILE]] [--runtime=docker|podman] <verb> <object> [args] [options]")
	warnln("Commands:")
	for _, line := range cmd.CommandRegistryTopLevelHelpLines() {
		warnf("  %s\n", line)
	}
}
