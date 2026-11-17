package composeutil

import (
	"fmt"
	"strconv"

	"github.com/compose-spec/compose-go/v2/types"
)

// EnvValue returns the string value for a key in MappingWithEquals, converting
// from *string. Returns ("", false) if absent or nil.
func EnvValue(env types.MappingWithEquals, key string) (string, bool) {
	v, ok := env[key]
	if !ok || v == nil {
		return "", false
	}
	return *v, true
}

// EnvAsMap converts MappingWithEquals (*string values) to map[string]string,
// dropping nil entries.
func EnvAsMap(env types.MappingWithEquals) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		if v != nil {
			out[k] = *v
		}
	}
	return out
}

// SetEnv sets a key in MappingWithEquals. The value is copied into a new
// string pointer.
func SetEnv(env types.MappingWithEquals, key, value string) {
	v := value
	env[key] = &v
}

// PublishedPort parses the Published field of a ServicePortConfig to an int.
// compose-go stores Published as a string to support ranges like "8080-8090".
// Returns 0, false if the field is empty or not a single integer.
func PublishedPort(p types.ServicePortConfig) (int, bool) {
	if p.Published == "" {
		return 0, false
	}
	n, err := strconv.Atoi(p.Published)
	if err != nil {
		return 0, false
	}
	return n, true
}

// MutateService retrieves the ServiceConfig for the named service, passes it
// to the mutate function, and writes the modified value back into
// project.Services. This is necessary because Services is a
// map[string]ServiceConfig (value type) — you cannot take the address of a
// map element in Go.
//
// Returns an error if the service does not exist.
func MutateService(project *types.Project, name string, mutate func(*types.ServiceConfig)) error {
	svc, ok := project.Services[name]
	if !ok {
		return fmt.Errorf("service %q not found in project", name)
	}
	mutate(&svc)
	project.Services[name] = svc
	return nil
}
