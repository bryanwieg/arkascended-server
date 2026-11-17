package main

import (
	"ark-cli/internal/cmd"
	"fmt"
	"strings"
)

type globalOptions struct {
	verbose bool
	logFile string
	help    bool
	version bool
	runtime string // "docker", "podman", or "" (auto-detect)
}

func parseGlobalOptions(args []string) (globalOptions, []string, error) {
	opts := globalOptions{}
	index := 0
	for index < len(args) {
		current := args[index]

		if current == "-h" || current == "--help" {
			opts.help = true
			return opts, nil, nil
		}
		if current == "--version" {
			opts.version = true
			return opts, nil, nil
		}
		if !strings.HasPrefix(current, "-") {
			return opts, args[index:], nil
		}

		if current == "-v" || current == "--verbose" {
			opts.verbose = true
			// Support `-v logfile` while keeping command detection unambiguous.
			if index+1 < len(args) {
				next := args[index+1]
				if !strings.HasPrefix(next, "-") && !cmd.IsCommandToken(next) {
					opts.logFile = next
					index += 2
					continue
				}
			}
			index++
			continue
		}
		if strings.HasPrefix(current, "-v=") {
			opts.verbose = true
			opts.logFile = strings.TrimPrefix(current, "-v=")
			index++
			continue
		}
		if strings.HasPrefix(current, "--verbose=") {
			opts.verbose = true
			opts.logFile = strings.TrimPrefix(current, "--verbose=")
			index++
			continue
		}

		if current == "--runtime" {
			if index+1 >= len(args) {
				return globalOptions{}, nil, fmt.Errorf("Error: --runtime requires a value (docker or podman)")
			}
			opts.runtime = args[index+1]
			index += 2
			continue
		}
		if strings.HasPrefix(current, "--runtime=") {
			opts.runtime = strings.TrimPrefix(current, "--runtime=")
			index++
			continue
		}

		return globalOptions{}, nil, fmt.Errorf("Error: unknown global option: %s", current)
	}
	return opts, args[index:], nil
}
