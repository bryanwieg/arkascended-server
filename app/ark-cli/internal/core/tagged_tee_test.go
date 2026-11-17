package core

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestTaggedTeeWriterSingleLine(t *testing.T) {
	var buf bytes.Buffer
	w := NewTaggedTeeWriter(&buf, "cli-stdout")
	n, err := w.Write([]byte("hello\n"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len("hello\n") {
		t.Errorf("Write returned %d; want %d", n, len("hello\n"))
	}
	if buf.String() != "[cli-stdout] hello\n" {
		t.Errorf("output mismatch; got %q", buf.String())
	}
}

func TestTaggedTeeWriterMultipleLinesPerWrite(t *testing.T) {
	var buf bytes.Buffer
	w := NewTaggedTeeWriter(&buf, "event")
	_, err := w.Write([]byte("one\ntwo\nthree\n"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := "[event] one\n[event] two\n[event] three\n"
	if buf.String() != want {
		t.Errorf("output mismatch;\ngot:  %q\nwant: %q", buf.String(), want)
	}
}

func TestTaggedTeeWriterPartialLineBuffered(t *testing.T) {
	var buf bytes.Buffer
	w := NewTaggedTeeWriter(&buf, "subprocess")
	// No newline yet; nothing should be emitted.
	if _, err := w.Write([]byte("partial ")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("partial line emitted prematurely: %q", buf.String())
	}
	// Newline arrives; full line emitted.
	if _, err := w.Write([]byte("line\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if buf.String() != "[subprocess] partial line\n" {
		t.Errorf("reassembled line mismatch; got %q", buf.String())
	}
}

func TestTaggedTeeWriterMixedCompleteAndPartial(t *testing.T) {
	var buf bytes.Buffer
	w := NewTaggedTeeWriter(&buf, "cli-stderr")
	if _, err := w.Write([]byte("first\nsecond")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// "first\n" emitted; "second" buffered.
	if buf.String() != "[cli-stderr] first\n" {
		t.Errorf("intermediate mismatch; got %q", buf.String())
	}
	if _, err := w.Write([]byte(" line\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := "[cli-stderr] first\n[cli-stderr] second line\n"
	if buf.String() != want {
		t.Errorf("final mismatch;\ngot:  %q\nwant: %q", buf.String(), want)
	}
}

func TestTaggedTeeWriterEmptyWrite(t *testing.T) {
	var buf bytes.Buffer
	w := NewTaggedTeeWriter(&buf, "event")
	n, err := w.Write(nil)
	if err != nil {
		t.Fatalf("Write(nil): %v", err)
	}
	if n != 0 {
		t.Errorf("Write(nil) returned %d; want 0", n)
	}
	if buf.Len() != 0 {
		t.Errorf("empty write produced output: %q", buf.String())
	}
}

func TestTaggedTeeWriterFlush(t *testing.T) {
	var buf bytes.Buffer
	w := NewTaggedTeeWriter(&buf, "subprocess")
	if _, err := w.Write([]byte("no newline")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("buffered line emitted without flush: %q", buf.String())
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if buf.String() != "[subprocess] no newline\n" {
		t.Errorf("flush mismatch; got %q", buf.String())
	}
	// Second flush with empty buffer is a no-op.
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush (empty): %v", err)
	}
	if buf.String() != "[subprocess] no newline\n" {
		t.Errorf("second flush added content; got %q", buf.String())
	}
}

func TestTaggedTeeWriterConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	w := NewTaggedTeeWriter(&buf, "event")
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			for range 50 {
				_, _ = w.Write([]byte("msg\n"))
			}
		}(i)
	}
	wg.Wait()

	// Every line should be a complete, well-formed tagged line.
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 500 {
		t.Errorf("expected 500 lines; got %d", len(lines))
	}
	for _, line := range lines {
		if line != "[event] msg" {
			t.Errorf("corrupt line; got %q", line)
			break
		}
	}
}

type errWriter struct{ err error }

func (e *errWriter) Write(_ []byte) (int, error) { return 0, e.err }

func TestTaggedTeeWriterPropagatesUnderlyingError(t *testing.T) {
	want := errors.New("underlying boom")
	w := NewTaggedTeeWriter(&errWriter{err: want}, "cli-stdout")
	_, err := w.Write([]byte("line\n"))
	if !errors.Is(err, want) {
		t.Errorf("Write err = %v; want %v", err, want)
	}
}

// TestTaggedTeeWriterIsIOWriter verifies the type satisfies io.Writer so
// it drops into existing APIs (slog.NewTextHandler, io.MultiWriter,
// exec.Cmd.Stdout) without adaptation.
func TestTaggedTeeWriterIsIOWriter(t *testing.T) {
	var _ io.Writer = (*TaggedTeeWriter)(nil)
}
