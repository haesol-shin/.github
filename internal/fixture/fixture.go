package fixture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type kv struct {
	key   string
	value any
}

// Load matches validate.py::load_fixture: recursive extends, JSON deep-copy,
// ordered dotted replace, and child expected extraction.
func Load(path string) (map[string]any, *string, error) {
	return load(path, map[string]struct{}{})
}

func load(path string, seen map[string]struct{}) (map[string]any, *string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	if _, ok := seen[abs]; ok {
		return nil, nil, fmt.Errorf("fixture cycle: %s", path)
	}
	seen[abs] = struct{}{}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	document, err := decodeJSON(data)
	if err != nil {
		return nil, nil, err
	}
	root, ok := document.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("%s: fixture root must be an object", path)
	}
	expected := expectedString(root)

	ext, hasExtends := root["extends"]
	if !hasExtends {
		return root, expected, nil
	}
	rel, ok := ext.(string)
	if !ok {
		return nil, nil, fmt.Errorf("%s: extends must be a string", path)
	}

	parent, _, err := load(filepath.Join(filepath.Dir(path), rel), seen)
	if err != nil {
		return nil, nil, err
	}
	state, ok := cloneJSON(parent).(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("%s: parent fixture is not an object", path)
	}

	if raw, ok := root["replace"]; ok && raw != nil {
		pairs, err := orderedReplace(data)
		if err != nil {
			return nil, nil, err
		}
		for _, pair := range pairs {
			if err := applyReplace(state, pair.key, pair.value); err != nil {
				return nil, nil, fmt.Errorf("%s: replace %q: %w", path, pair.key, err)
			}
		}
	}
	return state, expected, nil
}

func expectedString(doc map[string]any) *string {
	v, ok := doc["expected"]
	if !ok || v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return nil
	}
	return &s
}

func decodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("fixture contains more than one JSON value")
		}
		return nil, err
	}
	return convert(v), nil
}

func orderedReplace(data []byte) ([]kv, error) {
	var envelope struct {
		Replace json.RawMessage `json:"replace"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Replace) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Replace), []byte("null")) {
		return nil, nil
	}

	dec := json.NewDecoder(bytes.NewReader(envelope.Replace))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("replace must be an object")
	}

	var pairs []kv
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := kt.(string)
		if !ok {
			return nil, fmt.Errorf("replace key must be a string")
		}
		var value any
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		pairs = append(pairs, kv{key: key, value: convert(value)})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return pairs, nil
}

func convert(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t
	case map[string]any:
		for k, val := range t {
			t[k] = convert(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = convert(val)
		}
		return t
	default:
		return v
	}
}

func cloneJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = cloneJSON(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = cloneJSON(val)
		}
		return out
	default:
		return v
	}
}

func applyReplace(state map[string]any, dotted string, value any) error {
	parts := strings.Split(dotted, ".")
	var target any = state
	for _, part := range parts[:len(parts)-1] {
		next, err := traverse(target, part)
		if err != nil {
			return err
		}
		target = next
	}
	return assign(target, parts[len(parts)-1], value)
}

func traverse(target any, part string) (any, error) {
	switch t := target.(type) {
	case []any:
		i, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}
		if i < 0 || i >= len(t) {
			return nil, fmt.Errorf("index %d out of range", i)
		}
		return t[i], nil
	case map[string]any:
		v, ok := t[part]
		if !ok {
			return nil, fmt.Errorf("missing key %q", part)
		}
		return v, nil
	default:
		return nil, fmt.Errorf("cannot traverse %T", target)
	}
}

func assign(target any, final string, value any) error {
	switch t := target.(type) {
	case []any:
		i, err := strconv.Atoi(final)
		if err != nil {
			return err
		}
		if i < 0 || i >= len(t) {
			return fmt.Errorf("index %d out of range", i)
		}
		t[i] = value
		return nil
	case map[string]any:
		t[final] = value
		return nil
	default:
		return fmt.Errorf("cannot assign into %T", target)
	}
}
