package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Failure is one schema assertion failure. Identity is Label, InstancePath, and
// Keyword. InstancePath is a JSON Pointer (empty string at the instance root).
type Failure struct {
	Label        string
	InstancePath string
	Keyword      string
	Message      string
}

// ValidateFile loads a Draft 2020-12 schema from schemaPath and evaluates
// instance against the frozen keyword subset. Unknown keywords return an error
// instead of skipping those constraints.
func ValidateFile(instance any, schemaPath string, label string) ([]Failure, error) {
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, err
	}
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("decode schema %s: %w", schemaPath, err)
	}
	if err := checkSupported(schema, ""); err != nil {
		return nil, err
	}
	v := validator{label: label}
	v.apply(schema, instance, nil)
	sort.SliceStable(v.failures, func(i, j int) bool {
		a, b := v.failures[i], v.failures[j]
		if a.InstancePath != b.InstancePath {
			return a.InstancePath < b.InstancePath
		}
		if a.Keyword != b.Keyword {
			return a.Keyword < b.Keyword
		}
		return a.Message < b.Message
	})
	return v.failures, nil
}
