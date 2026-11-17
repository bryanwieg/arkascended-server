package runtime

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// ComposeDoc wraps a parsed compose YAML AST for editing.
type ComposeDoc struct {
	file *ast.File
	root *ast.MappingNode
}

// ParseComposeDoc parses compose YAML into an AST wrapper for edits.
func ParseComposeDoc(text string) (*ComposeDoc, error) {
	if strings.TrimSpace(text) == "" {
		text = "{}\n"
	}
	file, err := parser.ParseBytes([]byte(text), parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if len(file.Docs) > 1 {
		return nil, errf("Error: compose file must contain a single YAML document.")
	}
	root, err := RootMapping(file)
	if err != nil {
		return nil, err
	}
	return &ComposeDoc{file: file, root: root}, nil
}

// RootMapping returns the top-level mapping node, normalizing single-entry docs.
func RootMapping(file *ast.File) (*ast.MappingNode, error) {
	if file == nil || len(file.Docs) == 0 || file.Docs[0].Body == nil {
		return nil, errf("Error: compose file is empty.")
	}
	doc := file.Docs[0]
	if mapping, ok := doc.Body.(*ast.MappingNode); ok {
		return mapping, nil
	}
	if mappingValue, ok := doc.Body.(*ast.MappingValueNode); ok {
		mapping := ast.Mapping(mappingValue.GetToken(), false, mappingValue)
		doc.Body = mapping
		return mapping, nil
	}
	return nil, errf("Error: compose file must be a YAML mapping.")
}

// MapKeyString normalizes a map key node to its string form.
func MapKeyString(key ast.MapKeyNode) string {
	if key == nil {
		return ""
	}
	return strings.TrimSpace(key.String())
}

// MapValueForKey finds a mapping entry by key in a map node.
func MapValueForKey(node ast.MapNode, key string) (*ast.MappingValueNode, int) {
	switch mapping := node.(type) {
	case *ast.MappingNode:
		for idx, value := range mapping.Values {
			if MapKeyString(value.Key) == key {
				return value, idx
			}
		}
	case *ast.MappingValueNode:
		if MapKeyString(mapping.Key) == key {
			return mapping, 0
		}
	}
	return nil, -1
}

// MapValueForKeyFold finds a mapping entry by key using case-insensitive match.
func MapValueForKeyFold(node ast.MapNode, key string) (*ast.MappingValueNode, int) {
	switch mapping := node.(type) {
	case *ast.MappingNode:
		for idx, value := range mapping.Values {
			if strings.EqualFold(MapKeyString(value.Key), key) {
				return value, idx
			}
		}
	case *ast.MappingValueNode:
		if strings.EqualFold(MapKeyString(mapping.Key), key) {
			return mapping, 0
		}
	}
	return nil, -1
}

// MappingValues returns the mapping entries for either map node type.
func MappingValues(node ast.MapNode) []*ast.MappingValueNode {
	switch mapping := node.(type) {
	case *ast.MappingNode:
		return mapping.Values
	case *ast.MappingValueNode:
		return []*ast.MappingValueNode{mapping}
	default:
		return nil
	}
}

// ParseSnippetMappingValue parses a snippet and returns its mapping value.
func ParseSnippetMappingValue(snippet string, key string) (*ast.MappingValueNode, error) {
	file, err := parser.ParseBytes([]byte(snippet), parser.ParseComments)
	if err != nil {
		return nil, err
	}
	root, err := RootMapping(file)
	if err != nil {
		return nil, err
	}
	value, _ := MapValueForKey(root, key)
	if value == nil {
		return nil, errf("Error: snippet missing %s section.", key)
	}
	return value, nil
}

// EnsureMappingNode coerces a map value into a mapping node.
func EnsureMappingNode(value *ast.MappingValueNode) (*ast.MappingNode, error) {
	switch node := value.Value.(type) {
	case *ast.MappingNode:
		return node, nil
	case *ast.MappingValueNode:
		mapping := ast.Mapping(node.GetToken(), false, node)
		value.Value = mapping
		return mapping, nil
	default:
		return nil, errf("Error: section is not a mapping.")
	}
}

// String returns the serialized YAML for the compose document.
func (d *ComposeDoc) String() string {
	return d.file.String()
}

// MapValue returns the map entry for the given key.
func (d *ComposeDoc) MapValue(key string) (*ast.MappingValueNode, bool) {
	value, _ := MapValueForKey(d.root, key)
	return value, value != nil
}

// MapNode returns the map node stored at the key.
func (d *ComposeDoc) MapNode(key string) (ast.MapNode, bool) {
	value, ok := d.MapValue(key)
	if !ok {
		return nil, false
	}
	node, ok := value.Value.(ast.MapNode)
	return node, ok
}

// EnsureMap ensures a mapping exists for the key and returns it.
func (d *ComposeDoc) EnsureMap(key string) (*ast.MappingNode, error) {
	if value, ok := d.MapValue(key); ok {
		return EnsureMappingNode(value)
	}
	snippet := fmt.Sprintf("%s: {}\n", key)
	value, err := ParseSnippetMappingValue(snippet, key)
	if err != nil {
		return nil, err
	}
	mapping, err := EnsureMappingNode(value)
	if err != nil {
		return nil, errf("Error: %s section is not a mapping.", key)
	}
	mapping.SetIsFlowStyle(false)
	d.root.Values = append(d.root.Values, value)
	return mapping, nil
}

// AppendFromSnippet appends entries from a snippet into the target section.
func (d *ComposeDoc) AppendFromSnippet(key, snippet string) error {
	value, err := ParseSnippetMappingValue(snippet, key)
	if err != nil {
		return err
	}
	node, ok := value.Value.(ast.MapNode)
	if !ok {
		return errf("Error: %s section is not a mapping.", key)
	}
	values := MappingValues(node)
	if len(values) == 0 {
		return nil
	}
	mapping, err := d.EnsureMap(key)
	if err != nil {
		return err
	}
	mapping.Values = append(mapping.Values, values...)
	return nil
}

// RemoveEntry removes a named entry from a section map.
func (d *ComposeDoc) RemoveEntry(section, name string) bool {
	value, ok := d.MapValue(section)
	if !ok {
		return false
	}
	node, ok := value.Value.(ast.MapNode)
	if !ok {
		return false
	}
	_, idx := MapValueForKeyFold(node, name)
	if idx == -1 {
		return false
	}
	switch mapping := node.(type) {
	case *ast.MappingNode:
		mapping.Values = append(mapping.Values[:idx], mapping.Values[idx+1:]...)
		if len(mapping.Values) == 0 {
			value.Value = ast.Null(value.GetToken())
		}
		return true
	case *ast.MappingValueNode:
		value.Value = ast.Mapping(value.GetToken(), false)
		return true
	default:
		return false
	}
}

// ExtractServiceBlock returns the service section content from rendered YAML.
func ExtractServiceBlock(rendered string) (string, error) {
	doc, err := ParseComposeDoc(rendered)
	if err != nil {
		return "", err
	}
	value, ok := doc.MapValue("services")
	if !ok {
		return "", errf("Error: template missing services section.")
	}
	node, ok := value.Value.(ast.MapNode)
	if !ok {
		return "", errf("Error: services section is not a mapping.")
	}
	values := MappingValues(node)
	if len(values) == 0 {
		return "", nil
	}
	entries := make([]string, 0, len(values))
	for _, value := range values {
		entries = append(entries, value.String())
	}
	return strings.Join(entries, "\n") + "\n", nil
}

// InsertService inserts a rendered service block into the compose YAML.
func InsertService(composeText, serviceBlock string) string {
	doc, err := ParseComposeDoc(composeText)
	if err != nil {
		return composeText
	}
	snippet := "services:\n" + strings.TrimRight(serviceBlock, "\n") + "\n"
	if err := doc.AppendFromSnippet("services", snippet); err != nil {
		return composeText
	}
	if _, err := doc.EnsureMap("volumes"); err != nil {
		return doc.String()
	}
	return doc.String()
}

// EnsureVolumeEntry ensures a named volume exists in the compose YAML.
func EnsureVolumeEntry(composeText, volumeName string) string {
	doc, err := ParseComposeDoc(composeText)
	if err != nil {
		return composeText
	}
	volumes, err := doc.EnsureMap("volumes")
	if err != nil {
		return composeText
	}
	if existing, _ := MapValueForKeyFold(volumes, volumeName); existing != nil {
		return doc.String()
	}
	snippet := fmt.Sprintf("volumes:\n  %s:\n", volumeName)
	if err := doc.AppendFromSnippet("volumes", snippet); err != nil {
		return composeText
	}
	return doc.String()
}

// EnsureServiceAbsent errors if the compose YAML already has the service.
func EnsureServiceAbsent(composeText, instance string) error {
	doc, err := ParseComposeDoc(composeText)
	if err != nil {
		return err
	}
	value, ok := doc.MapValue("services")
	if !ok {
		return nil
	}
	node, ok := value.Value.(ast.MapNode)
	if !ok {
		return errf("Error: services section is not a mapping.")
	}
	if value, _ := MapValueForKeyFold(node, instance); value != nil {
		return errf("Error: service already exists in compose file: %s", instance)
	}
	return nil
}

// RemoveServiceBlock removes the named service from the compose YAML.
func RemoveServiceBlock(composeText, instance string) (string, bool) {
	doc, err := ParseComposeDoc(composeText)
	if err != nil {
		return composeText, false
	}
	removed := doc.RemoveEntry("services", instance)
	if !removed {
		return composeText, false
	}
	return doc.String(), true
}

// RemoveVolumeEntry removes the named volume from the compose YAML.
func RemoveVolumeEntry(composeText, volumeName string) (string, bool) {
	doc, err := ParseComposeDoc(composeText)
	if err != nil {
		return composeText, false
	}
	removed := doc.RemoveEntry("volumes", volumeName)
	if !removed {
		return composeText, false
	}
	return doc.String(), true
}

// errf formats a compose-editing error message.
func errf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}
