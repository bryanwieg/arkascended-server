package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := InstanceState{
		PreviousImage: "ark-server:100",
		LastAttempt: &UpdateAttempt{
			Time: time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC), From: "ark-server:100", To: "ark-server:200",
			Result: ResultRolledBack, Message: "not ready", Backup: "/b/a.tar.gz",
		},
	}
	if err := SaveState(dir, "asa1", want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(dir, "asa1")
	if err != nil {
		t.Fatal(err)
	}
	if got.PreviousImage != want.PreviousImage || *got.LastAttempt != *want.LastAttempt {
		t.Fatalf("got %+v %+v", got, got.LastAttempt)
	}
}

func TestLoadStateMissingIsZero(t *testing.T) {
	st, err := LoadState(t.TempDir(), "asa1")
	if err != nil || st.PreviousImage != "" || st.LastAttempt != nil {
		t.Fatalf("got %+v, %v", st, err)
	}
	all, err := LoadAllStates(t.TempDir())
	if err != nil || len(all) != 0 {
		t.Fatalf("got %v, %v", all, err)
	}
}

func TestLoadStateInvalidJSONNamesFile(t *testing.T) {
	dir := t.TempDir()
	path := StatePath(dir, "asa1")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("{nope"), 0o644)
	if _, err := LoadState(dir, "asa1"); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("err = %v, want the file path", err)
	}
	if _, err := LoadAllStates(dir); err == nil {
		t.Fatal("LoadAllStates must fail on a corrupt file")
	}
}

func TestLoadAllStates(t *testing.T) {
	dir := t.TempDir()
	SaveState(dir, "asa1", InstanceState{PreviousImage: "ark-server:1"})
	SaveState(dir, "asa2", InstanceState{PreviousImage: "ark-server:2"})
	all, err := LoadAllStates(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all["asa1"].PreviousImage != "ark-server:1" || all["asa2"].PreviousImage != "ark-server:2" {
		t.Fatalf("got %v", all)
	}
}
