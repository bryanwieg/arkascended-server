package cmd

// Tests: command registry integrity: verb sets, parse specs, route-to-handler coverage, help line sort order

import (
	"strings"
	"testing"
)

func TestCommandRegistryVerbSetMatchesGlobalCommandNames(t *testing.T) {
	registryVerbs := CommandRegistryVerbSet()
	for verb := range CommandTokenSet() {
		if _, ok := registryVerbs[verb]; !ok {
			t.Fatalf("CommandTokenSet has unexpected extra verb %q not present in registry", verb)
		}
	}
	for verb := range registryVerbs {
		if _, ok := CommandTokenSet()[verb]; !ok {
			t.Fatalf("registry missing verb %q present in CommandTokenSet", verb)
		}
	}
}

func TestCommandRegistryVerbsHaveParseSpecs(t *testing.T) {
	for _, entry := range CommandRegistry {
		if _, ok := CommandRegistryParseSpec(entry.Verb); !ok {
			t.Fatalf("missing parse spec for verb %q", entry.Verb)
		}
	}
}

func TestCommandRegistryParseSpecsMatchKnownVerbs(t *testing.T) {
	registryVerbs := CommandRegistryVerbSet()
	for verb := range CommandRegistryVerbParseSpecs {
		if _, ok := registryVerbs[verb]; !ok {
			t.Fatalf("parse spec exists for unknown verb %q", verb)
		}
	}
}

func TestCommandRegistryRoutesHaveHandlers(t *testing.T) {
	for _, entry := range CommandRegistry {
		if entry.Handler == nil {
			t.Fatalf("missing route handler for route %q (verb=%q object=%q)", entry.Route, entry.Verb, entry.Object)
		}
		if _, ok := CommandRegistryRouteHandler(entry.Route); !ok {
			t.Fatalf("route handler lookup missing route %q (verb=%q object=%q)", entry.Route, entry.Verb, entry.Object)
		}
	}
}

func TestCommandRegistryRouteHandlerRejectsUnknownRoute(t *testing.T) {
	if _, ok := CommandRegistryRouteHandler("missing_route"); ok {
		t.Fatal("expected missing route handler lookup to fail")
	}
}

func TestCommandRegistryTopLevelHelpLinesSortedByVerbAndObject(t *testing.T) {
	lines := CommandRegistryTopLevelHelpLines()
	prevVerb := ""
	prevObject := ""
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			t.Fatalf("unexpected help line format: %q", line)
		}
		verb := fields[0]
		object := fields[1]
		if prevVerb > verb || (prevVerb == verb && prevObject > object) {
			t.Fatalf("usage help lines out of verb/object order: prev=%q/%q current=%q/%q", prevVerb, prevObject, verb, object)
		}
		prevVerb = verb
		prevObject = object
	}
}
