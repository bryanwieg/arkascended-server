package sdkbackend

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"

	"pkg/ark/runtime"
)

// Saves edit the compose file in place: only the image, environment, labels and ports entries
// that differ from the file are rewritten, line by line, and every other byte is kept. Re-marshaling
// the loaded project instead would bake in .env values and env_file secrets, make paths absolute,
// drop comments, and delete services whose profile is inactive.

var errUnsupportedSave = errors.New("ark-cli can only change image, environment, labels and ports")

// lineEdits collects replacements (start line → end line and new lines) and insertions after a line.
type lineEdits struct {
	replace map[int]replacement
	insert  map[int][]string
}

type replacement struct {
	end   int
	lines []string
}

// patchComposeText returns text with the image, environment, label and port differences between
// before (the project loaded from text) and after applied as line edits.
func patchComposeText(text string, before, after *types.Project) (string, error) {
	if err := checkSupportedChanges(before, after); err != nil {
		return "", err
	}
	file, err := parser.ParseBytes([]byte(text), parser.ParseComments)
	if err != nil {
		return "", err
	}
	root, err := runtime.RootMapping(file)
	if err != nil {
		return "", err
	}
	servicesEntry, _ := runtime.MapValueForKey(root, "services")
	if servicesEntry == nil {
		return "", errors.New("compose file has no services section")
	}
	services, err := blockMapping(servicesEntry.Value, "services")
	if err != nil {
		return "", err
	}

	lines := strings.Split(text, "\n")
	edits := &lineEdits{replace: map[int]replacement{}, insert: map[int][]string{}}
	for _, name := range slices.Sorted(maps.Keys(after.Services)) {
		if err := diffService(lines, edits, services, name, before.Services[name], after.Services[name]); err != nil {
			return "", fmt.Errorf("service %s: %w", name, err)
		}
	}
	return edits.apply(lines), nil
}

// checkSupportedChanges rejects changes outside the fields patchComposeText can edit.
func checkSupportedChanges(before, after *types.Project) error {
	if !slices.Equal(slices.Sorted(maps.Keys(before.Services)), slices.Sorted(maps.Keys(after.Services))) {
		return fmt.Errorf("%w; services were added or removed", errUnsupportedSave)
	}
	for name, a := range after.Services {
		b := before.Services[name]
		if !reflect.DeepEqual(withoutEditableFields(b), withoutEditableFields(a)) {
			return fmt.Errorf("%w; service %s changed other settings", errUnsupportedSave, name)
		}
		if a.Image == "" && b.Image != "" {
			return fmt.Errorf("%w; service %s lost its image", errUnsupportedSave, name)
		}
		for k := range b.Environment {
			if _, ok := a.Environment[k]; !ok {
				return fmt.Errorf("%w; service %s removed environment variable %s", errUnsupportedSave, name, k)
			}
		}
		for k := range b.Labels {
			if _, ok := a.Labels[k]; !ok {
				return fmt.Errorf("%w; service %s removed label %s", errUnsupportedSave, name, k)
			}
		}
		if len(a.Ports) < len(b.Ports) {
			return fmt.Errorf("%w; service %s removed a port", errUnsupportedSave, name)
		}
	}
	return nil
}

// verifySaved checks that the edited file loads to the project the caller asked for.
func verifySaved(want, got *types.Project) error {
	for name, w := range want.Services {
		g, ok := got.Services[name]
		switch {
		case !ok:
			return fmt.Errorf("service %s is missing after the edit", name)
		case !reflect.DeepEqual(withoutEditableFields(w), withoutEditableFields(g)):
			return fmt.Errorf("service %s changed other settings after the edit", name)
		case w.Image != g.Image, !reflect.DeepEqual(w.Environment, g.Environment), !reflect.DeepEqual(w.Labels, g.Labels):
			return fmt.Errorf("service %s does not have the requested image, environment or labels after the edit", name)
		case len(w.Ports) != len(g.Ports):
			return fmt.Errorf("service %s does not have the requested ports after the edit", name)
		}
		for i := range w.Ports {
			if portTuple(w.Ports[i]) != portTuple(g.Ports[i]) {
				return fmt.Errorf("service %s does not have the requested ports after the edit", name)
			}
		}
	}
	return nil
}

func withoutEditableFields(s types.ServiceConfig) types.ServiceConfig {
	s.Image, s.Environment, s.Labels, s.Ports = "", nil, nil, nil
	s.CustomLabels = nil // derived by the loader from the file's path, not stored in the file
	return s
}

// portKey is the part of a port binding that saves preserve; Mode and Name are not rewritten.
type portKey struct {
	hostIP, published, protocol string
	target                      uint32
}

func portTuple(p types.ServicePortConfig) portKey {
	return portKey{p.HostIP, p.Published, cmp.Or(p.Protocol, "tcp"), p.Target}
}

func diffService(lines []string, edits *lineEdits, services *ast.MappingNode, name string, b, a types.ServiceConfig) error {
	entry, _ := runtime.MapValueForKey(services, name)
	if entry == nil {
		return errors.New("not found in the compose file")
	}
	svc, err := blockMapping(entry.Value, name)
	if err != nil {
		return err
	}
	keyIndent := column(svc.Values[0].Key) - 1
	svcEnd := entryEnd(lines, line(entry.Key), column(entry.Key)-1)

	if a.Image != b.Image {
		value := strings.Repeat(" ", keyIndent) + "image: " + yamlScalar(a.Image)
		if err := setEntry(lines, edits, svc, "image", value, svcEnd); err != nil {
			return err
		}
	}
	for _, k := range slices.Sorted(maps.Keys(a.Environment)) {
		if old, ok := b.Environment[k]; !ok || !reflect.DeepEqual(old, a.Environment[k]) {
			if err := setKeyValue(lines, edits, svc, keyIndent, svcEnd, "environment", k, a.Environment[k]); err != nil {
				return err
			}
		}
	}
	for _, k := range slices.Sorted(maps.Keys(a.Labels)) {
		if old, ok := b.Labels[k]; !ok || old != a.Labels[k] {
			v := a.Labels[k]
			if err := setKeyValue(lines, edits, svc, keyIndent, svcEnd, "labels", k, &v); err != nil {
				return err
			}
		}
	}
	return diffPorts(lines, edits, svc, keyIndent, svcEnd, b.Ports, a.Ports)
}

// setEntry replaces the entry key with newLine, or inserts it at the end of the service.
func setEntry(lines []string, edits *lineEdits, svc *ast.MappingNode, key, newLine string, svcEnd int) error {
	entry, _ := runtime.MapValueForKey(svc, key)
	if entry == nil {
		edits.insert[svcEnd] = append(edits.insert[svcEnd], newLine)
		return nil
	}
	start := line(entry.Key)
	edits.replace[start] = replacement{end: entryEnd(lines, start, column(entry.Key)-1), lines: []string{newLine}}
	return nil
}

// setKeyValue sets key in a service's environment or labels section, in list or map form.
func setKeyValue(lines []string, edits *lineEdits, svc *ast.MappingNode, keyIndent, svcEnd int, section, key string, value *string) error {
	listItem := key
	if value != nil {
		listItem = key + "=" + escapeDollar(*value)
	}
	entry, _ := runtime.MapValueForKey(svc, section)
	if entry == nil {
		indent := strings.Repeat(" ", keyIndent)
		edits.insert[svcEnd] = append(edits.insert[svcEnd], indent+section+":", indent+"  - "+yamlScalar(listItem))
		return nil
	}
	switch node := entry.Value.(type) {
	case *ast.SequenceNode:
		if node.IsFlowStyle || len(node.Values) == 0 {
			return fmt.Errorf("%s is not a block list; edit it by hand", section)
		}
		dashIndent := column(node) - 1
		newLine := strings.Repeat(" ", dashIndent) + "- " + yamlScalar(listItem)
		lastEnd := 0
		for _, item := range node.Values {
			start := itemStart(lines, line(item), dashIndent)
			end := entryEnd(lines, start, dashIndent)
			lastEnd = end
			if k, _, _ := strings.Cut(item.GetToken().Value, "="); k == key {
				edits.replace[start] = replacement{end: end, lines: []string{newLine}}
				return nil
			}
		}
		edits.insert[lastEnd] = append(edits.insert[lastEnd], newLine)
		return nil
	default:
		mapping, err := blockMapping(entry.Value, section)
		if err != nil {
			return err
		}
		indent := column(mapping.Values[0].Key) - 1
		rendered := strings.Repeat(" ", indent) + yamlScalar(key) + ":"
		if value != nil {
			rendered += " " + yamlScalar(escapeDollar(*value))
		}
		lastEnd := 0
		for _, mv := range mapping.Values {
			start := line(mv.Key)
			end := entryEnd(lines, start, indent)
			lastEnd = end
			if mv.Key.GetToken().Value == key {
				edits.replace[start] = replacement{end: end, lines: []string{rendered}}
				return nil
			}
		}
		edits.insert[lastEnd] = append(edits.insert[lastEnd], rendered)
		return nil
	}
}

func diffPorts(lines []string, edits *lineEdits, svc *ast.MappingNode, keyIndent, svcEnd int, before, after []types.ServicePortConfig) error {
	changed := len(after) != len(before)
	for i := range before {
		changed = changed || portTuple(before[i]) != portTuple(after[i])
	}
	if !changed {
		return nil
	}
	entry, _ := runtime.MapValueForKey(svc, "ports")
	if entry == nil {
		indent := strings.Repeat(" ", keyIndent)
		edits.insert[svcEnd] = append(edits.insert[svcEnd], indent+"ports:")
		for _, p := range after {
			edits.insert[svcEnd] = append(edits.insert[svcEnd], indent+"  - "+shortPort(p))
		}
		return nil
	}
	seq, ok := entry.Value.(*ast.SequenceNode)
	if !ok || seq.IsFlowStyle || len(seq.Values) != len(before) {
		return errors.New("ports use flow style or port ranges, which ark-cli cannot edit; edit them by hand")
	}
	dashIndent := strings.Repeat(" ", column(seq)-1)
	lastEnd := 0
	for i, item := range seq.Values {
		start := itemStart(lines, line(item), column(seq)-1)
		end := entryEnd(lines, start, column(seq)-1)
		lastEnd = end
		if portTuple(before[i]) != portTuple(after[i]) {
			edits.replace[start] = replacement{end: end, lines: []string{dashIndent + "- " + shortPort(after[i])}}
		}
	}
	for _, p := range after[len(before):] {
		edits.insert[lastEnd] = append(edits.insert[lastEnd], dashIndent+"- "+shortPort(p))
	}
	return nil
}

// shortPort renders a binding in the quoted short syntax the template uses.
func shortPort(p types.ServicePortConfig) string {
	var b strings.Builder
	if p.HostIP != "" {
		if strings.Contains(p.HostIP, ":") {
			b.WriteString("[" + p.HostIP + "]:")
		} else {
			b.WriteString(p.HostIP + ":")
		}
	}
	if p.Published != "" {
		b.WriteString(p.Published + ":")
	}
	b.WriteString(strconv.FormatUint(uint64(p.Target), 10))
	if p.Protocol != "" {
		b.WriteString("/" + p.Protocol)
	}
	return `"` + b.String() + `"`
}

func (e *lineEdits) apply(lines []string) string {
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if r, ok := e.replace[i]; ok {
			out = append(out, r.lines...)
			out = append(out, e.insert[r.end]...)
			i = r.end
			continue
		}
		out = append(out, lines[i])
		out = append(out, e.insert[i]...)
	}
	return strings.Join(out, "\n")
}

// entryEnd returns the last line (0-based) of the block entry starting at start: following lines
// indented deeper than indent, ignoring trailing blank and comment-only lines.
func entryEnd(lines []string, start, indent int) int {
	// A list may sit at its key's indent ("ports:\n- x"); its items still belong to the key.
	keyWithBlock := strings.HasSuffix(strings.TrimSpace(lines[start]), ":")
	last := start
	for i := start + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		ind := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
		listAtKeyIndent := keyWithBlock && strings.HasPrefix(trimmed, "-")
		if ind < indent || ind == indent && !listAtKeyIndent {
			break
		}
		last = i
	}
	return last
}

// itemStart returns the line of the "-" that starts the list item whose value is on valueLine.
func itemStart(lines []string, valueLine, dashIndent int) int {
	for i := valueLine; i >= 0; i-- {
		l := lines[i]
		if len(l) > dashIndent && strings.HasPrefix(l[dashIndent:], "-") && strings.TrimSpace(l[:dashIndent]) == "" {
			return i
		}
	}
	return valueLine
}

func blockMapping(node ast.Node, name string) (*ast.MappingNode, error) {
	m, ok := node.(*ast.MappingNode)
	if !ok {
		if mv, isValue := node.(*ast.MappingValueNode); isValue {
			return ast.Mapping(mv.GetToken(), false, mv), nil
		}
		return nil, fmt.Errorf("%s is not a mapping", name)
	}
	if m.IsFlowStyle {
		return nil, fmt.Errorf("%s uses flow style ({...}), which ark-cli cannot edit; edit it by hand", name)
	}
	if len(m.Values) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}
	return m, nil
}

func line(n ast.Node) int   { return n.GetToken().Position.Line - 1 }
func column(n ast.Node) int { return n.GetToken().Position.Column }

// escapeDollar keeps compose from interpolating a literal value.
func escapeDollar(s string) string { return strings.ReplaceAll(s, "$", "$$") }

// yamlScalar writes s plain when YAML reads it back as the same string, else double-quoted.
func yamlScalar(s string) string {
	letter, plain := false, s != "" && !strings.ContainsAny(s[:1], "-.:@=") && !strings.HasSuffix(s, ":")
	for _, r := range s {
		isLetter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		letter = letter || isLetter
		plain = plain && (isLetter || r >= '0' && r <= '9' || strings.ContainsRune("_-./=@:", r))
	}
	if plain && letter && !isYAMLKeyword(s) {
		return s
	}
	return fmt.Sprintf("%q", s)
}

func isYAMLKeyword(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "on", "off", "null", "~", "y", "n":
		return true
	}
	return false
}
