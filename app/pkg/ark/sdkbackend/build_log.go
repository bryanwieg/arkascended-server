package sdkbackend

import (
	"encoding/json"
	"log/slog"
	"strings"

	controlapi "github.com/moby/buildkit/api/services/control"
	"github.com/moby/moby/api/types/jsonstream"
	"google.golang.org/protobuf/proto"
)

const buildLogTailLines = 20

// buildLog turns BuildKit's progress trace into log lines. The API sends each step's output
// as protobuf StatusResponse messages inside "moby.buildkit.trace" aux messages; without
// decoding them a failed build reports only "did not complete successfully: exit code: 1".
// Every output line goes to the debug log (ark-cli -v), and the last lines are kept for the
// error message.
type buildLog struct {
	logger  *slog.Logger
	names   map[string]string // vertex digest -> step name, e.g. "[ark_download 5/5] RUN ..."
	partial map[string]string // vertex digest -> output not yet ended by a newline
	tail    []string
}

func newBuildLog(logger *slog.Logger) *buildLog {
	return &buildLog{logger: logger, names: map[string]string{}, partial: map[string]string{}}
}

// handle is the aux callback for jsonmessage.DisplayJSONMessagesStream.
func (b *buildLog) handle(msg jsonstream.Message) {
	if msg.ID != "moby.buildkit.trace" || msg.Aux == nil {
		return
	}
	var raw []byte
	if err := json.Unmarshal(*msg.Aux, &raw); err != nil {
		return
	}
	var status controlapi.StatusResponse
	if err := proto.Unmarshal(raw, &status); err != nil {
		return
	}
	for _, v := range status.GetVertexes() {
		b.names[v.GetDigest()] = v.GetName()
		if v.GetError() != "" {
			b.add(v.GetDigest(), "ERROR: "+v.GetError())
		}
	}
	for _, l := range status.GetLogs() {
		text := b.partial[l.GetVertex()] + string(l.GetMsg())
		lines := strings.Split(text, "\n")
		b.partial[l.GetVertex()] = lines[len(lines)-1]
		for _, line := range lines[:len(lines)-1] {
			b.add(l.GetVertex(), line)
		}
	}
}

// flush emits output that never ended with a newline.
func (b *buildLog) flush() {
	for vertex, text := range b.partial {
		if text != "" {
			b.add(vertex, text)
		}
	}
	clear(b.partial)
}

func (b *buildLog) add(vertex, line string) {
	line = strings.TrimRight(line, "\r")
	b.logger.Debug("build output", "step", b.names[vertex], "line", line)
	b.tail = append(b.tail, line)
	if len(b.tail) > buildLogTailLines {
		b.tail = b.tail[len(b.tail)-buildLogTailLines:]
	}
}

// tailText returns the last output lines, for appending to a build error.
func (b *buildLog) tailText() string {
	b.flush()
	return strings.Join(b.tail, "\n")
}
