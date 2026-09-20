package collect

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func DecodePolicy(text string) (any, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	var out any
	if err := yaml.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("repository policy YAML: %w", err)
	}
	return convertYAML(out), nil
}

func convertYAML(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = convertYAML(val)
		}
		return t
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			key, ok := k.(string)
			if !ok {
				key = fmt.Sprint(k)
			}
			out[key] = convertYAML(val)
		}
		return out
	case []any:
		for i, val := range t {
			t[i] = convertYAML(val)
		}
		return t
	default:
		return v
	}
}
