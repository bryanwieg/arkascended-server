package runtime

// Tests: compose YAML AST editing: service block extraction and insertion, volumes section creation

import (
	"strings"
	"testing"
)

// TestExtractServiceBlock pulls the service section out of a rendered template.
// - Expect: block excludes top-level services/volumes headers.
func TestExtractServiceBlock(t *testing.T) {
	rendered := "services:\n  asa1:\n    image: ark-server:latest\nvolumes:\n  asa1-saved:\n"
	block, err := ExtractServiceBlock(rendered)
	if err != nil {
		t.Fatalf("ExtractServiceBlock: %v", err)
	}
	if strings.Contains(block, "services:") || strings.Contains(block, "volumes:") {
		t.Fatal("expected only service block content")
	}
	if !strings.Contains(block, "asa1:") {
		t.Fatal("expected service content in block")
	}
}

// TestInsertServiceAddsService verifies InsertService adds a new service block.
// - Expect: compose output contains the new service name.
func TestInsertServiceAddsService(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\nvolumes:\n  asa1-saved:\n"
	serviceBlock := "  asa2:\n    image: ark-server:latest\n"
	updated := InsertService(composeText, serviceBlock)
	if !strings.Contains(updated, "asa2:") {
		t.Fatal("expected asa2 to be added")
	}
}

// TestInsertServiceAddsVolumesSection verifies volumes are added when missing.
// - Expect: volumes header is appended after insert.
func TestInsertServiceAddsVolumesSection(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\n"
	serviceBlock := "  asa2:\n    image: ark-server:latest\n"
	updated := InsertService(composeText, serviceBlock)
	if !strings.Contains(updated, "volumes:") {
		t.Fatal("expected volumes section to be added")
	}
}

// TestInsertServiceWithInlineSection verifies inline services are expanded safely.
// - Expect: inline services map expanded to include new service.
func TestInsertServiceWithInlineSection(t *testing.T) {
	composeText := "services: {}\nvolumes: {}\n"
	serviceBlock := "  asa2:\n    image: ark-server:latest\n"
	updated := InsertService(composeText, serviceBlock)
	if !strings.Contains(updated, "asa2:") {
		t.Fatal("expected asa2 to be added")
	}
}

// TestInsertServiceBeforeNextSection keeps next section comments attached.
// - Expect: new service appears before the comment that precedes the next section.
func TestInsertServiceBeforeNextSection(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\n# volumes are defined below\nvolumes:\n  asa1-saved:\n"
	serviceBlock := "  asa2:\n    image: ark-server:latest\n"
	updated := InsertService(composeText, serviceBlock)

	commentIdx := strings.Index(updated, "# volumes are defined below")
	serviceIdx := strings.Index(updated, "asa2:")
	volumesIdx := strings.Index(updated, "volumes:")
	if serviceIdx == -1 || commentIdx == -1 || volumesIdx == -1 {
		t.Fatal("expected service, comment, and volumes to be present")
	}
	if !(serviceIdx < commentIdx && commentIdx < volumesIdx) {
		t.Fatalf("expected service before comment before volumes, got %d < %d < %d", serviceIdx, commentIdx, volumesIdx)
	}
}

// TestEnsureVolumeEntryAddsMissing verifies EnsureVolumeEntry adds missing volume entries.
// - Expect: compose output contains the requested volume.
func TestEnsureVolumeEntryAddsMissing(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\nvolumes:\n  asa1-saved:\n"
	updated := EnsureVolumeEntry(composeText, "asa2-saved")
	if !strings.Contains(updated, "asa2-saved:") {
		t.Fatal("expected volume entry")
	}
}

// TestEnsureVolumeEntryWithInlineSection verifies inline volumes are expanded safely.
// - Expect: inline volumes map expanded with new entry.
func TestEnsureVolumeEntryWithInlineSection(t *testing.T) {
	composeText := "services: {}\nvolumes: {}\n"
	updated := EnsureVolumeEntry(composeText, "asa2-saved")
	if !strings.Contains(updated, "asa2-saved:") {
		t.Fatal("expected volume entry")
	}
}

// TestRemoveServiceBlockWithInlineSection ensures inline services remove cleanly.
// - Expect: target service removed and other service remains.
func TestRemoveServiceBlockWithInlineSection(t *testing.T) {
	composeText := "services: {asa1: {image: ark-server:latest}, asa2: {image: ark-server:latest}}\n"
	updated, removed := RemoveServiceBlock(composeText, "asa1")
	if !removed {
		t.Fatal("expected removal")
	}
	if strings.Contains(updated, "asa1:") {
		t.Fatal("expected asa1 removed")
	}
	if !strings.Contains(updated, "asa2:") {
		t.Fatal("expected asa2 still present")
	}
}

// TestInsertServicePreservesTopLevelComment keeps top-level comments when inserting.
// - Expect: comment remains after insert.
func TestInsertServicePreservesTopLevelComment(t *testing.T) {
	composeText := "# compose file\nservices:\n  asa1:\n    image: ark-server:latest\n"
	serviceBlock := "  asa2:\n    image: ark-server:latest\n"
	updated := InsertService(composeText, serviceBlock)
	if !strings.Contains(updated, "# compose file") {
		t.Fatal("expected top-level comment preserved")
	}
	if !strings.Contains(updated, "asa2:") {
		t.Fatal("expected asa2 to be added")
	}
}

// TestEnsureServiceAbsentRejectsNonMapping ensures non-map services error.
// - Expect: error when services is not a mapping.
func TestEnsureServiceAbsentRejectsNonMapping(t *testing.T) {
	composeText := "services: []\n"
	if err := EnsureServiceAbsent(composeText, "asa1"); err == nil {
		t.Fatal("expected error")
	}
}

// TestEnsureVolumeEntryWithFlowStyleSection verifies flow style volumes are expanded.
// - Expect: new volume is added.
func TestEnsureVolumeEntryWithFlowStyleSection(t *testing.T) {
	composeText := "volumes: {asa1-saved: {}}\n"
	updated := EnsureVolumeEntry(composeText, "asa2-saved")
	if !strings.Contains(updated, "asa2-saved:") {
		t.Fatal("expected volume entry")
	}
}

// TestEnsureVolumeEntryRejectsNonMapping ensures non-map volumes are ignored.
// - Expect: compose text unchanged when volumes is not a mapping.
func TestEnsureVolumeEntryRejectsNonMapping(t *testing.T) {
	composeText := "volumes: []\n"
	updated := EnsureVolumeEntry(composeText, "asa2-saved")
	if updated != composeText {
		t.Fatal("expected compose text unchanged")
	}
}

// TestEnsureServiceAbsentIsCaseInsensitive verifies service matching ignores case.
// - Expect: error when service differs only by case.
func TestEnsureServiceAbsentIsCaseInsensitive(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\n"
	if err := EnsureServiceAbsent(composeText, "ASA1"); err == nil {
		t.Fatal("expected error")
	}
}

// TestInsertServiceNoopOnMultiDoc ensures multi-doc YAML is rejected.
// - Expect: compose text unchanged.
func TestInsertServiceNoopOnMultiDoc(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\n---\nfoo: bar\n"
	serviceBlock := "  asa2:\n    image: ark-server:latest\n"
	updated := InsertService(composeText, serviceBlock)
	if updated != composeText {
		t.Fatal("expected compose text unchanged")
	}
}

// TestEnsureServiceAbsentRejectsMultiDoc ensures multi-doc YAML errors.
// - Expect: error for multi-document input.
func TestEnsureServiceAbsentRejectsMultiDoc(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\n---\nfoo: bar\n"
	if err := EnsureServiceAbsent(composeText, "asa1"); err == nil {
		t.Fatal("expected error")
	}
}

// TestEnsureVolumeEntryCaseInsensitive ensures volume matching ignores case.
// - Expect: existing volume (case-insensitive) prevents duplicates.
func TestEnsureVolumeEntryCaseInsensitive(t *testing.T) {
	composeText := "volumes:\n  Asa2-saved:\n"
	updated := EnsureVolumeEntry(composeText, "asa2-saved")
	if strings.Count(updated, "asa2-saved:")+strings.Count(updated, "Asa2-saved:") != 1 {
		t.Fatal("expected single volume entry")
	}
}

// TestEnsureServiceAbsentIgnoresVolumes ensures volume names do not block services.
// - Expect: no error when only a volume uses the name.
func TestEnsureServiceAbsentIgnoresVolumes(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\nvolumes:\n  asa2:\n"
	if err := EnsureServiceAbsent(composeText, "asa2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestEnsureServiceAbsentRejectsExisting ensures EnsureServiceAbsent errors on duplicates.
// - Expect: error when service already exists.
func TestEnsureServiceAbsentRejectsExisting(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\n"
	if err := EnsureServiceAbsent(composeText, "asa1"); err == nil {
		t.Fatal("expected error")
	}
}

// TestExtractServiceBlockStopsAtNextSection verifies extraction stops before the next section.
// - Expect: block excludes networks section.
func TestExtractServiceBlockStopsAtNextSection(t *testing.T) {
	rendered := "services:\n  asa1:\n    image: ark-server:latest\n# network settings below\nnetworks:\n  arknet:\n"
	block, err := ExtractServiceBlock(rendered)
	if err != nil {
		t.Fatalf("ExtractServiceBlock: %v", err)
	}
	if strings.Contains(block, "networks:") {
		t.Fatal("expected networks section to be excluded")
	}
}

// TestRemoveServiceBlockRemovesService verifies RemoveServiceBlock deletes the target service only.
// - Expect: target service absent and other services remain.
func TestRemoveServiceBlockRemovesService(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\n  asa2:\n    image: ark-server:latest\nvolumes:\n  asa1-saved:\n"
	updated, removed := RemoveServiceBlock(composeText, "asa1")
	if !removed {
		t.Fatal("expected removal")
	}
	if strings.Contains(updated, "asa1:") {
		t.Fatal("expected asa1 removed")
	}
	if !strings.Contains(updated, "asa2:") {
		t.Fatal("expected asa2 still present")
	}
}

// TestRemoveServiceBlockLastServiceLeavesEmptyMapping keeps the file loadable after the last service goes.
// - Expect: `services: {}` (block and inline forms), never `services: null`, which compose refuses.
func TestRemoveServiceBlockLastServiceLeavesEmptyMapping(t *testing.T) {
	for _, composeText := range []string{
		"services:\n  asa1:\n    image: ark-server:latest\nvolumes:\n  ark-cluster-1:\n",
		"services: {asa1: {image: ark-server:latest}}\n",
	} {
		updated, removed := RemoveServiceBlock(composeText, "asa1")
		if !removed || !strings.Contains(updated, "services: {}") {
			t.Fatalf("removed=%v, got:\n%s", removed, updated)
		}
	}
}

// TestRemoveServiceBlockMissingIsNoop ensures RemoveServiceBlock is a no-op when service is absent.
// - Expect: RemoveServiceBlock returns false without altering compose.
func TestRemoveServiceBlockMissingIsNoop(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\n"
	updated, removed := RemoveServiceBlock(composeText, "missing")
	if removed {
		t.Fatal("expected no removal")
	}
	if updated != composeText {
		t.Fatal("expected compose text unchanged")
	}
}

// TestRemoveVolumeEntryRemovesBlock verifies RemoveVolumeEntry deletes nested entries.
// - Expect: volume entry and options are removed from compose output.
func TestRemoveVolumeEntryRemovesBlock(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\nvolumes:\n  asa1-saved:\n    external: true\n    name: asa1-saved\n  asa2-saved:\n"
	updated, removed := RemoveVolumeEntry(composeText, "asa1-saved")
	if !removed {
		t.Fatal("expected removal")
	}
	if strings.Contains(updated, "asa1-saved") || strings.Contains(updated, "external: true") {
		t.Fatal("expected volume block removed")
	}
	if !strings.Contains(updated, "asa2-saved:") {
		t.Fatal("expected other volume to remain")
	}
}

// TestRemoveVolumeEntryRemovesEntry verifies RemoveVolumeEntry deletes a volume entry.
// - Expect: volume entry is removed from compose output.
func TestRemoveVolumeEntryRemovesEntry(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\nvolumes:\n  asa1-saved:\n  asa2-saved:\n"
	updated, removed := RemoveVolumeEntry(composeText, "asa1-saved")
	if !removed {
		t.Fatal("expected removal")
	}
	if strings.Contains(updated, "asa1-saved") {
		t.Fatal("expected volume removed")
	}
	if !strings.Contains(updated, "asa2-saved:") {
		t.Fatal("expected other volume to remain")
	}
}

// TestRemoveVolumeEntryMissingIsNoop ensures RemoveVolumeEntry is a no-op when volume is absent.
// - Expect: RemoveVolumeEntry returns false without altering compose.
func TestRemoveVolumeEntryMissingIsNoop(t *testing.T) {
	composeText := "services:\n  asa1:\n    image: ark-server:latest\n"
	updated, removed := RemoveVolumeEntry(composeText, "asa1-saved")
	if removed {
		t.Fatal("expected no removal")
	}
	if updated != composeText {
		t.Fatal("expected compose text unchanged")
	}
}
