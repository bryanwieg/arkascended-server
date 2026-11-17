package main

import (
	"testing"
)

func TestParseGlobalOptionsRuntimeFlag(t *testing.T) {
	opts, remaining, err := parseGlobalOptions([]string{"--runtime", "podman", "create", "instance"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if opts.runtime != "podman" {
		t.Fatalf("expected runtime=podman, got %q", opts.runtime)
	}
	if len(remaining) != 2 || remaining[0] != "create" {
		t.Fatalf("unexpected remaining: %#v", remaining)
	}
}

func TestParseGlobalOptionsRuntimeEquals(t *testing.T) {
	opts, remaining, err := parseGlobalOptions([]string{"--runtime=docker", "start", "asa1"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if opts.runtime != "docker" {
		t.Fatalf("expected runtime=docker, got %q", opts.runtime)
	}
	if len(remaining) != 2 || remaining[0] != "start" {
		t.Fatalf("unexpected remaining: %#v", remaining)
	}
}

func TestParseGlobalOptionsRuntimeMissingValue(t *testing.T) {
	_, _, err := parseGlobalOptions([]string{"--runtime"})
	if err == nil {
		t.Fatal("expected error for --runtime without value")
	}
}
func TestParseGlobalOptionsVersion(t *testing.T) {
	opts, remaining, err := parseGlobalOptions([]string{"--version", "start", "instance"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if !opts.version || remaining != nil {
		t.Fatalf("expected version=true and no remaining args, got %v %#v", opts.version, remaining)
	}
}
