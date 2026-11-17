package cmd

import (
	"ark-cli/internal/core"
	"sort"
	"strings"
)

var CommandRegistryVerbParseSpecs = buildCommandRegistryVerbParseSpecs()
var CommandRegistryTokenNames = CommandRegistryVerbSet()
var CommandRegistryLookupByVerbObject = buildCommandRegistryLookupByVerbObject()
var CommandRegistryHandlersByRoute = buildCommandRegistryHandlersByRoute()
var CommandRegistryTopHelpLines = buildCommandRegistryTopLevelHelpLines()

func CommandRegistryVerbSet() map[string]struct{} {
	verbs := map[string]struct{}{}
	for _, entry := range CommandRegistry {
		verbs[entry.Verb] = struct{}{}
	}
	return verbs
}

func CommandRegistryParseSpec(verb string) (CommandRouteParseSpec, bool) {
	spec, ok := CommandRegistryVerbParseSpecs[verb]
	return spec, ok
}

func buildCommandRegistryVerbParseSpecs() map[string]CommandRouteParseSpec {
	verbObjects := map[string][]string{}
	seenVerbObjects := map[string]map[string]struct{}{}
	for _, entry := range CommandRegistry {
		if _, ok := seenVerbObjects[entry.Verb]; !ok {
			seenVerbObjects[entry.Verb] = map[string]struct{}{}
		}
		if _, exists := seenVerbObjects[entry.Verb][entry.Object]; exists {
			continue
		}
		seenVerbObjects[entry.Verb][entry.Object] = struct{}{}
		verbObjects[entry.Verb] = append(verbObjects[entry.Verb], entry.Object)
	}

	specs := map[string]CommandRouteParseSpec{}
	for verb, objects := range verbObjects {
		if len(objects) == 0 {
			continue
		}
		specs[verb] = CommandRouteParseSpec{
			MissingErr: "Error: " + verb + " expected object: " + naturalList(objects) + ".",
			WrongFmt:   "Error: " + verb + " expected object " + naturalQuotedList(objects) + ", got: %s",
		}
	}
	return specs
}

func naturalList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " or " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", or " + items[len(items)-1]
	}
}

func naturalQuotedList(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, "'"+item+"'")
	}
	return naturalList(quoted)
}

func CommandTokenSet() map[string]struct{} {
	return CommandRegistryTokenNames
}

func IsCommandToken(token string) bool {
	_, ok := CommandTokenSet()[token]
	return ok
}

func CommandRegistryLookup(verb, object string) (CommandRegistryEntry, bool) {
	entry, ok := CommandRegistryLookupByVerbObject[verb+" "+object]
	return entry, ok
}

func CommandRegistryRouteHandler(route string) (func([]string) error, bool) {
	handler, ok := CommandRegistryHandlersByRoute[route]
	return handler, ok
}

func CommandRegistryTopLevelHelpLines() []string {
	return CommandRegistryTopHelpLines
}

func buildCommandRegistryTopLevelHelpLines() []string {
	type helpLine struct {
		verb   string
		object string
		text   string
	}
	lines := []helpLine{}
	for _, entry := range CommandRegistry {
		if len(entry.TopLevelHelp) == 0 {
			continue
		}
		for _, line := range entry.TopLevelHelp {
			lines = append(lines, helpLine{verb: entry.Verb, object: entry.Object, text: line})
		}
	}
	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].verb != lines[j].verb {
			return lines[i].verb < lines[j].verb
		}
		if lines[i].object != lines[j].object {
			return lines[i].object < lines[j].object
		}
		return false
	})
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, line.text)
	}
	return result
}

func buildCommandRegistryLookupByVerbObject() map[string]CommandRegistryEntry {
	lookup := make(map[string]CommandRegistryEntry, len(CommandRegistry))
	for _, entry := range CommandRegistry {
		lookup[entry.Verb+" "+entry.Object] = entry
	}
	return lookup
}

func buildCommandRegistryHandlersByRoute() map[string]func([]string) error {
	handlers := make(map[string]func([]string) error, len(CommandRegistry))
	for _, entry := range CommandRegistry {
		h := entry.Handler
		handlers[entry.Route] = func(args []string) error {
			return h(core.CurrentAppDeps(), args)
		}
	}
	return handlers
}
