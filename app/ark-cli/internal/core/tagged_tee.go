package core

import (
	"bytes"
	"io"
	"sync"
)

// TaggedTeeWriter wraps an io.Writer and prefixes every newline-terminated
// line with "[tag] ". Partial lines (bytes without a trailing newline) are
// buffered until a newline arrives, so a writer that emits in chunks
// still produces one tagged line per logical line.
//
// Intended as the shared mechanism for the verbose log file tee
// (docs/dev/explanation/logging-architecture.md#log-file-tee): every
// source of tee'd output wraps its destination with a TaggedTeeWriter
// so the file reads as one chronologically-ordered, channel-tagged
// stream. Example tags: "cli-stdout", "cli-stderr", "event", "json",
// "subprocess", "docker-logs".
//
// Safe for concurrent Write calls from multiple goroutines.
type TaggedTeeWriter struct {
	mu  sync.Mutex
	out io.Writer
	tag []byte // pre-rendered "[tag] " prefix
	buf bytes.Buffer
}

// NewTaggedTeeWriter returns a TaggedTeeWriter that writes tagged lines
// to out. An empty tag is permitted but produces "[] " which is almost
// never what you want.
func NewTaggedTeeWriter(out io.Writer, tag string) *TaggedTeeWriter {
	return &TaggedTeeWriter{
		out: out,
		tag: []byte("[" + tag + "] "),
	}
}

// Write appends p to the internal line buffer, emits every complete line
// prefixed with the tag, and returns len(p), err where err comes from the
// underlying writer. A single Write call may produce multiple tagged
// lines in the output if p contains multiple newlines, and may produce
// zero output if p has no newline (the partial line is buffered).
func (w *TaggedTeeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writeLocked(p)
}

func (w *TaggedTeeWriter) writeLocked(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		data := w.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}
		line := data[:idx+1]
		if _, err := w.out.Write(w.tag); err != nil {
			// Discard the partial-line buffer on underlying-writer
			// error; returning mid-line would leave us in an
			// inconsistent state on the next call.
			w.buf.Reset()
			return len(p), err
		}
		if _, err := w.out.Write(line); err != nil {
			w.buf.Reset()
			return len(p), err
		}
		// Drop the emitted bytes from the buffer.
		remaining := make([]byte, len(data)-(idx+1))
		copy(remaining, data[idx+1:])
		w.buf.Reset()
		w.buf.Write(remaining)
	}
	return len(p), nil
}

// Flush emits any buffered partial line as a tagged line with an
// appended newline. Call at known breakpoints (end of a subprocess,
// end of a request) to avoid indefinite buffering when a source
// writes bytes without newline termination.
func (w *TaggedTeeWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf.Len() == 0 {
		return nil
	}
	if _, err := w.out.Write(w.tag); err != nil {
		w.buf.Reset()
		return err
	}
	if _, err := w.out.Write(w.buf.Bytes()); err != nil {
		w.buf.Reset()
		return err
	}
	if _, err := w.out.Write([]byte{'\n'}); err != nil {
		w.buf.Reset()
		return err
	}
	w.buf.Reset()
	return nil
}
