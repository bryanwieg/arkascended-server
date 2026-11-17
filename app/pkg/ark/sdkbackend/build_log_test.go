package sdkbackend

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	controlapi "github.com/moby/buildkit/api/services/control"
	"github.com/moby/moby/api/types/jsonstream"
	"google.golang.org/protobuf/proto"
)

// traceMessage encodes status the way the daemon does: proto bytes, JSON-encoded into Aux.
func traceMessage(t *testing.T, status *controlapi.StatusResponse) jsonstream.Message {
	t.Helper()
	raw, err := proto.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	aux, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	msg := json.RawMessage(aux)
	return jsonstream.Message{ID: "moby.buildkit.trace", Aux: &msg}
}

func TestBuildLogKeepsStepOutputForTheError(t *testing.T) {
	var debug bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&debug, &slog.HandlerOptions{Level: slog.LevelDebug}))
	b := newBuildLog(logger)

	step := "[ark_download 5/5] RUN ark-server-install.sh"
	b.handle(traceMessage(t, &controlapi.StatusResponse{
		Vertexes: []*controlapi.Vertex{{Digest: "sha256:a", Name: step}},
	}))
	// Output arrives in arbitrary chunks; a line may span two messages.
	b.handle(traceMessage(t, &controlapi.StatusResponse{
		Logs: []*controlapi.VertexLog{{Vertex: "sha256:a", Msg: []byte("Update state (0x5) verifying\nSuccess! App '24")}},
	}))
	b.handle(traceMessage(t, &controlapi.StatusResponse{
		Logs: []*controlapi.VertexLog{{Vertex: "sha256:a", Msg: []byte("30919' fully installed.\r\nwrite /data: no space left on device")}},
	}))
	b.handle(traceMessage(t, &controlapi.StatusResponse{
		Vertexes: []*controlapi.Vertex{{Digest: "sha256:a", Name: step, Error: "exit code: 1"}},
	}))
	// Non-trace messages and undecodable aux data are ignored, not fatal.
	bad := json.RawMessage(`"not-proto"`)
	b.handle(jsonstream.Message{ID: "moby.buildkit.trace", Aux: &bad})
	b.handle(jsonstream.Message{Stream: "plain stream text\n"})

	want := "Update state (0x5) verifying\n" +
		"Success! App '2430919' fully installed.\n" +
		"ERROR: exit code: 1\n" +
		"write /data: no space left on device"
	if got := b.tailText(); got != want {
		t.Fatalf("tail:\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(debug.String(), "no space left on device") || !strings.Contains(debug.String(), "ark_download 5/5") {
		t.Fatalf("debug log lacks the step output or step name:\n%s", debug.String())
	}
}

func TestBuildLogTailIsBounded(t *testing.T) {
	b := newBuildLog(slog.New(slog.DiscardHandler))
	var many strings.Builder
	for i := range 50 {
		many.WriteString("line " + string(rune('A'+i%26)) + "\n")
	}
	b.handle(traceMessage(t, &controlapi.StatusResponse{
		Logs: []*controlapi.VertexLog{{Vertex: "v", Msg: []byte(many.String())}},
	}))
	if got := strings.Count(b.tailText(), "\n") + 1; got != buildLogTailLines {
		t.Fatalf("expected %d tail lines, got %d", buildLogTailLines, got)
	}
}
