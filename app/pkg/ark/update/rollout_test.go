package update

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeSteps records calls; fail maps a call label ("stop", "backup", "set:<image>",
// "start:<n>" for the n-th start, "restore") to the error it returns.
type fakeSteps struct {
	calls  []string
	fail   map[string]error
	starts int
}

func (f *fakeSteps) steps() RolloutSteps {
	call := func(label string) error {
		f.calls = append(f.calls, label)
		return f.fail[label]
	}
	return RolloutSteps{
		Stop: func(context.Context, string) error { return call("stop") },
		Backup: func(context.Context, string) (string, error) {
			return "/backups/pre-update.tar.gz", call("backup")
		},
		SetImage: func(_ context.Context, _, image string) error { return call("set:" + image) },
		StartAndWait: func(context.Context, string) error {
			f.starts++
			return call("start:" + string(rune('0'+f.starts)))
		},
		Restore: func(_ context.Context, _, archive string) error { return call("restore:" + archive) },
		Now:     func() time.Time { return time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC) },
	}
}

func TestRollout(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name       string
		fail       map[string]error
		wantCalls  string
		wantResult UpdateResult // "" = nothing recorded
		wantErr    string       // "" = nil; "rolled back" = wraps ErrRolledBack
		wantPrev   string
	}{
		{name: "success", wantCalls: "stop backup set:new start:1", wantResult: ResultSucceeded, wantPrev: "old"},
		{name: "stop fails", fail: map[string]error{"stop": boom}, wantCalls: "stop", wantErr: "could not stop"},
		{name: "backup fails", fail: map[string]error{"backup": boom}, wantCalls: "stop backup start:1", wantResult: ResultRolledBack, wantErr: "rolled back", wantPrev: "keep"},
		{name: "backup and restart fail", fail: map[string]error{"backup": boom, "start:1": boom}, wantCalls: "stop backup start:1", wantResult: ResultRollbackFailed, wantErr: "not rolled back", wantPrev: "keep"},
		{name: "new image not ready", fail: map[string]error{"start:1": boom},
			wantCalls:  "stop backup set:new start:1 set:old stop restore:/backups/pre-update.tar.gz start:2",
			wantResult: ResultRolledBack, wantErr: "rolled back", wantPrev: "old"},
		{name: "restore fails", fail: map[string]error{"start:1": boom, "restore:/backups/pre-update.tar.gz": boom},
			wantCalls:  "stop backup set:new start:1 set:old stop restore:/backups/pre-update.tar.gz",
			wantResult: ResultRollbackFailed, wantErr: "not rolled back", wantPrev: "old"},
		{name: "old image fails to start", fail: map[string]error{"start:1": boom, "start:2": boom},
			wantCalls:  "stop backup set:new start:1 set:old stop restore:/backups/pre-update.tar.gz start:2",
			wantResult: ResultRollbackFailed, wantErr: "rollback failed at starting old", wantPrev: "old"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := SaveState(dir, "asa1", InstanceState{PreviousImage: "keep"}); err != nil {
				t.Fatal(err)
			}
			f := &fakeSteps{fail: tc.fail}
			attempt, err := Rollout(context.Background(), RolloutRequest{StateDir: dir, Instance: "asa1", From: "old", To: "new"}, f.steps())

			if got := strings.Join(f.calls, " "); got != tc.wantCalls {
				t.Fatalf("calls %q, want %q", got, tc.wantCalls)
			}
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr == "rolled back" && !errors.Is(err, ErrRolledBack):
				t.Fatalf("err = %v, want ErrRolledBack", err)
			case tc.wantErr != "" && tc.wantErr != "rolled back" && (err == nil || !strings.Contains(err.Error(), tc.wantErr) || errors.Is(err, ErrRolledBack)):
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
			st, loadErr := LoadState(dir, "asa1")
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if tc.wantResult == "" {
				if st.LastAttempt != nil {
					t.Fatalf("recorded %+v, want nothing", st.LastAttempt)
				}
				return
			}
			if st.LastAttempt == nil || st.LastAttempt.Result != tc.wantResult || attempt.Result != tc.wantResult {
				t.Fatalf("recorded %+v, returned %+v, want %s", st.LastAttempt, attempt, tc.wantResult)
			}
			if st.PreviousImage != tc.wantPrev {
				t.Fatalf("previous image %q, want %q", st.PreviousImage, tc.wantPrev)
			}
			if tc.wantResult != ResultSucceeded && st.LastAttempt.Message == "" {
				t.Fatal("a failed attempt needs a message")
			}
		})
	}
}
