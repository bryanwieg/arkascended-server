package schedule

// JSON output purity tests for schedule commands.
// See docs/dev/explanation/logging-architecture.md#json-output-contract.
// These tests guard the untested JSON output surface identified in the
// logging audit (Gap 8): any regression that emits stray text alongside
// the JSON document, or breaks the shape of list/show JSON output,
// fails here before reaching a downstream script consumer.

import (
	"encoding/json"
	"strings"
	"testing"

	"ark-cli/internal/core"
)

// TestListSchedulesJSONEmpty verifies that `list schedules --json`
// with no schedules configured emits `[]`, not `null` and not the
// human-readable "No schedules configured." message.
func TestListSchedulesJSONEmpty(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, "version: 1\nschedules: []\n")

	out := captureStdout(t, func() {
		if err := RunListSchedules(core.CurrentAppDeps(), []string{"--json"}); err != nil {
			t.Fatalf("RunListSchedules: %v", err)
		}
	})

	// Must parse as JSON.
	var parsed []core.ScheduleEntryJSON
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout:\n%s", err, out)
	}
	if len(parsed) != 0 {
		t.Errorf("expected empty array; got %d entries", len(parsed))
	}
	// Must not contain the text-mode empty message.
	if strings.Contains(out, "No schedules configured") {
		t.Errorf("JSON output contains text-mode message; channels crossed.\nstdout:\n%s", out)
	}
}

// TestListSchedulesJSONShape verifies that `list schedules --json`
// emits a valid JSON array whose entries match the documented schema.
// Tests the shape of the JSON output across every expected field.
func TestListSchedulesJSONShape(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `version: 1
schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
  - name: weekly
    type: backup
    instance: asa2
    enabled: false
    cron: "0 4 * * 0"
`)

	out := captureStdout(t, func() {
		if err := RunListSchedules(core.CurrentAppDeps(), []string{"--json"}); err != nil {
			t.Fatalf("RunListSchedules: %v", err)
		}
	})

	var parsed []core.ScheduleEntryJSON
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout:\n%s", err, out)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 entries; got %d\nstdout:\n%s", len(parsed), out)
	}

	// First entry shape.
	if parsed[0].Name != "nightly" || parsed[0].Type != "backup" ||
		parsed[0].Instance != "asa1" || !parsed[0].Enabled ||
		parsed[0].Cron != "0 3 * * *" {
		t.Errorf("entry[0] shape unexpected: %+v", parsed[0])
	}
	if parsed[1].Name != "weekly" || parsed[1].Enabled {
		t.Errorf("entry[1] shape unexpected: %+v", parsed[1])
	}

	// The JSON output must not carry text-mode table headers.
	for _, forbidden := range []string{"NAME", "ENABLED", "LAST_STATUS"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("JSON output contains text-mode table header %q; channels crossed.\nstdout:\n%s", forbidden, out)
		}
	}
}

// TestShowScheduleJSON verifies that `show schedule <name> --json`
// emits a valid ScheduleDetailJSON document and does not mix text-mode
// YAML-ish detail lines into the JSON stream.
func TestShowScheduleJSON(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `version: 1
schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
    format: tar.gz
    retain_count: 20
    retain_days: 14
    retain_size_gb: 5
`)

	out := captureStdout(t, func() {
		if err := RunShowSchedule(core.CurrentAppDeps(), []string{"--name", "nightly", "--json"}); err != nil {
			t.Fatalf("RunShowSchedule: %v", err)
		}
	})

	var parsed core.ScheduleDetailJSON
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout:\n%s", err, out)
	}
	if parsed.Name != "nightly" || parsed.Type != "backup" ||
		parsed.Instance != "asa1" || !parsed.Enabled ||
		parsed.Cron != "0 3 * * *" || parsed.Format != "tar.gz" ||
		parsed.RetainCount != 20 || parsed.RetainDays != 14 ||
		parsed.RetainSizeGB != 5 {
		t.Errorf("ScheduleDetailJSON shape unexpected: %+v", parsed)
	}

	// JSON must not leak the text-mode YAML-ish detail lines.
	for _, forbidden := range []string{"name: nightly", "enabled: yes", "command_preview:"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("JSON output contains text-mode detail line %q; channels crossed.\nstdout:\n%s", forbidden, out)
		}
	}
}

// TestListSchedulesTextModeUnchanged verifies that text mode still emits
// the documented headers. This catches a regression where a JSON change
// accidentally reroutes the text-mode output.
func TestListSchedulesTextModeUnchanged(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `version: 1
schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)

	out := captureStdout(t, func() {
		if err := RunListSchedules(core.CurrentAppDeps(), nil); err != nil {
			t.Fatalf("RunListSchedules: %v", err)
		}
	})

	// Must not be JSON.
	trimmed := strings.TrimSpace(out)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		t.Errorf("text mode emitted JSON; channels crossed.\nstdout:\n%s", out)
	}
	// Must have the documented table header.
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "LAST_STATUS") {
		t.Errorf("text mode missing documented table headers.\nstdout:\n%s", out)
	}
}
