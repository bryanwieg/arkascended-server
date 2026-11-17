package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// ParsePort parses and validates a port number string.
func ParsePort(value, label string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("Error: %s must be an integer.", label)
	}
	if port <= 0 || port > 65535 {
		return 0, fmt.Errorf("Error: %s must be between 1 and 65535.", label)
	}
	return port, nil
}

// ParseNonNegativeInt parses and validates a non-negative integer string.
func ParseNonNegativeInt(value, label string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("Error: %s must be an integer.", label)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("Error: %s must be zero or greater.", label)
	}
	return parsed, nil
}

var composeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ARK server passwords only allow ASCII letters and numbers.
var passwordPattern = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

// ValidateComposeName checks that a compose name is valid.
func ValidateComposeName(value, label string) error {
	if value == "" {
		return fmt.Errorf("Error: %s is required.", label)
	}
	if !composeNamePattern.MatchString(value) {
		return fmt.Errorf("Error: %s must start with an alphanumeric character and contain only letters, numbers, underscores, dots, or dashes.", label)
	}
	return nil
}

// ValidateYAMLScalar checks that a value is safe for YAML scalars.
func ValidateYAMLScalar(value, label string) error {
	if strings.ContainsAny(value, "\n\r\t") {
		return fmt.Errorf("Error: %s cannot include newlines or tabs.", label)
	}
	if strings.Contains(value, ": ") {
		return fmt.Errorf("Error: %s cannot include ': ' (colon followed by space).", label)
	}
	if strings.Contains(value, " #") {
		return fmt.Errorf("Error: %s cannot include ' #' (space followed by #).", label)
	}
	return nil
}

// ValidatePassword checks that a password contains only letters and numbers.
func ValidatePassword(value, label string) error {
	if !passwordPattern.MatchString(value) {
		return fmt.Errorf("Error: %s must contain only letters and numbers.", label)
	}
	return nil
}

// MapNameMissingWP returns true if the map name does not end in _WP.
func MapNameMissingWP(mapName string) bool {
	return !strings.HasSuffix(mapName, "_WP")
}

// StripLineQuotes removes surrounding single or double quotes from a trimmed line.
func StripLineQuotes(s string) string {
	if len(s) >= 2 {
		if (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) || (strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// ParseHostPortFromLine extracts the published host port from a port line for the given protocol.
// Returns (port, true) on success.
func ParseHostPortFromLine(line, proto string) (int, bool) {
	raw := strings.TrimSpace(line)
	if strings.HasPrefix(raw, "-") {
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "-"))
	}
	raw = StripLineQuotes(raw)
	if !strings.HasSuffix(raw, "/"+proto) || !strings.Contains(raw, ":") {
		return 0, false
	}
	portPart := strings.SplitN(raw, "/", 2)[0]
	parts := strings.Split(portPart, ":")
	hostPart := parts[0]
	if len(parts) >= 2 {
		hostPart = parts[len(parts)-2]
	}
	if hostPart == "" {
		return 0, false
	}
	port, err := strconv.Atoi(hostPart)
	if err != nil {
		return 0, false
	}
	return port, true
}

// FindUsedPorts scans compose text for ports of the given protocol.
func FindUsedPorts(composeText, proto string) map[int]struct{} {
	used := make(map[int]struct{})
	for _, line := range strings.Split(composeText, "\n") {
		if port, ok := ParseHostPortFromLine(line, proto); ok {
			used[port] = struct{}{}
		}
	}
	return used
}

// PickNextPort finds the first available port starting from base.
func PickNextPort(used map[int]struct{}, base int) int {
	port := base
	for {
		if _, ok := used[port]; !ok {
			return port
		}
		port++
	}
}

// ParseComposeText parses compose YAML text into a map.
func ParseComposeText(text string) (map[string]interface{}, error) {
	if strings.TrimSpace(text) == "" {
		return map[string]interface{}{}, nil
	}
	var raw interface{}
	if err := yaml.Unmarshal([]byte(text), &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return map[string]interface{}{}, nil
	}
	normalized := NormalizeYAML(raw)
	result, ok := normalized.(map[string]interface{})
	if !ok {
		return nil, errors.New("Error: compose YAML must be a mapping.")
	}
	return result, nil
}

// ComposeToString serializes a compose map back to YAML text.
func ComposeToString(compose map[string]interface{}) (string, error) {
	data, err := yaml.Marshal(compose)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", nil
	}
	if data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return string(data), nil
}

// ComposeProjectName returns the Docker Compose project name for a given compose file.
// It respects the COMPOSE_PROJECT_NAME environment variable, falling back to the
// parent directory name of the resolved compose file path.
func ComposeProjectName(composeFile string) string {
	name := os.Getenv("COMPOSE_PROJECT_NAME")
	if name != "" {
		return name
	}
	if absComposeFile, err := filepath.Abs(composeFile); err == nil {
		return filepath.Base(filepath.Dir(absComposeFile))
	}
	return filepath.Base(filepath.Dir(composeFile))
}

// NormalizeYAML recursively normalizes YAML values (interface{} maps → string maps).
func NormalizeYAML(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, val := range typed {
			out[key] = NormalizeYAML(val)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, val := range typed {
			out[fmt.Sprint(key)] = NormalizeYAML(val)
		}
		return out
	case []interface{}:
		for i, val := range typed {
			typed[i] = NormalizeYAML(val)
		}
		return typed
	default:
		return value
	}
}
