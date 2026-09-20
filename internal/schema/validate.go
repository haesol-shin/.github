package schema

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var allowedKeywords = map[string]struct{}{
	"$schema":              {},
	"$id":                  {},
	"$comment":             {},
	"title":                {},
	"description":          {},
	"type":                 {},
	"properties":           {},
	"required":             {},
	"additionalProperties": {},
	"const":                {},
	"enum":                 {},
	"pattern":              {},
	"minLength":            {},
	"minimum":              {},
	"items":                {},
	"minItems":             {},
	"allOf":                {},
	"if":                   {},
	"then":                 {},
	"else":                 {},
	"not":                  {},
}

var allowedTypes = map[string]struct{}{
	"object":  {},
	"array":   {},
	"string":  {},
	"integer": {},
	"boolean": {},
}

type validator struct {
	label    string
	failures []Failure
}

func (v *validator) add(path []string, keyword, message string) {
	v.failures = append(v.failures, Failure{
		Label:        v.label,
		InstancePath: encodePointer(path),
		Keyword:      keyword,
		Message:      message,
	})
}

func encodePointer(path []string) string {
	if len(path) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range path {
		b.WriteByte('/')
		b.WriteString(escapePointer(part))
	}
	return b.String()
}

func escapePointer(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}

func checkSupported(schema any, loc string) error {
	obj, ok := schema.(map[string]any)
	if !ok {
		return fmt.Errorf("schema at %s must be an object", displayLoc(loc))
	}
	for key, value := range obj {
		if _, ok := allowedKeywords[key]; !ok {
			return fmt.Errorf("unsupported schema keyword %q at %s", key, displayLoc(loc))
		}
		child := loc + "/" + escapePointer(key)
		switch key {
		case "type":
			typ, ok := value.(string)
			if !ok {
				return fmt.Errorf("type must be a string at %s", displayLoc(loc))
			}
			if _, ok := allowedTypes[typ]; !ok {
				return fmt.Errorf("unsupported type %q at %s", typ, displayLoc(loc))
			}
		case "properties":
			props, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("properties must be an object at %s", displayLoc(loc))
			}
			for name, sub := range props {
				if err := checkSupported(sub, child+"/"+escapePointer(name)); err != nil {
					return err
				}
			}
		case "required":
			if err := requireStringArray(value, "required", loc); err != nil {
				return err
			}
		case "additionalProperties":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("additionalProperties must be a boolean at %s", displayLoc(loc))
			}
		case "enum":
			if _, ok := value.([]any); !ok {
				return fmt.Errorf("enum must be an array at %s", displayLoc(loc))
			}
		case "pattern":
			pat, ok := value.(string)
			if !ok {
				return fmt.Errorf("pattern must be a string at %s", displayLoc(loc))
			}
			if _, err := regexp.Compile(pat); err != nil {
				return fmt.Errorf("invalid pattern at %s: %w", displayLoc(loc), err)
			}
		case "minLength", "minItems":
			if _, ok := asNonNegativeInt(value); !ok {
				return fmt.Errorf("%s must be a non-negative integer at %s", key, displayLoc(loc))
			}
		case "minimum":
			if _, ok := asFloat(value); !ok {
				return fmt.Errorf("minimum must be a number at %s", displayLoc(loc))
			}
		case "items", "if", "then", "else", "not":
			if err := checkSupported(value, child); err != nil {
				return err
			}
		case "allOf":
			arr, ok := value.([]any)
			if !ok {
				return fmt.Errorf("allOf must be an array at %s", displayLoc(loc))
			}
			for i, sub := range arr {
				if err := checkSupported(sub, child+"/"+strconv.Itoa(i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func requireStringArray(value any, keyword, loc string) error {
	arr, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s must be an array at %s", keyword, displayLoc(loc))
	}
	for _, item := range arr {
		if _, ok := item.(string); !ok {
			return fmt.Errorf("%s entries must be strings at %s", keyword, displayLoc(loc))
		}
	}
	return nil
}

func displayLoc(loc string) string {
	if loc == "" {
		return "/"
	}
	return loc
}

func (v *validator) apply(schema any, instance any, path []string) {
	obj, ok := schema.(map[string]any)
	if !ok {
		return
	}
	if typ, ok := obj["type"].(string); ok {
		if !matchesType(typ, instance) {
			v.add(path, "type", fmt.Sprintf("expected %s, got %s", typ, typeName(instance)))
		}
	}
	if _, ok := obj["const"]; ok {
		if !jsonEqual(obj["const"], instance) {
			v.add(path, "const", "value does not equal const")
		}
	}
	if raw, ok := obj["enum"]; ok {
		options, _ := raw.([]any)
		found := false
		for _, option := range options {
			if jsonEqual(option, instance) {
				found = true
				break
			}
		}
		if !found {
			v.add(path, "enum", "value is not one of the enum values")
		}
	}
	if pat, ok := obj["pattern"].(string); ok {
		if s, ok := instance.(string); ok {
			re := regexp.MustCompile(pat)
			if !re.MatchString(s) {
				v.add(path, "pattern", "value does not match pattern")
			}
		}
	}
	if raw, ok := obj["minLength"]; ok {
		if s, ok := instance.(string); ok {
			min, _ := asNonNegativeInt(raw)
			if utf8.RuneCountInString(s) < min {
				v.add(path, "minLength", fmt.Sprintf("string length is less than %d", min))
			}
		}
	}
	if raw, ok := obj["minimum"]; ok {
		if n, ok := asFloat(instance); ok {
			min, _ := asFloat(raw)
			if n < min {
				v.add(path, "minimum", fmt.Sprintf("value is less than %v", raw))
			}
		}
	}
	if raw, ok := obj["minItems"]; ok {
		if arr, ok := instance.([]any); ok {
			min, _ := asNonNegativeInt(raw)
			if len(arr) < min {
				v.add(path, "minItems", fmt.Sprintf("array length is less than %d", min))
			}
		}
	}
	if raw, ok := obj["items"]; ok {
		if arr, ok := instance.([]any); ok {
			for i, item := range arr {
				v.apply(raw, item, append(path, strconv.Itoa(i)))
			}
		}
	}
	if raw, ok := obj["properties"]; ok {
		if m, ok := instance.(map[string]any); ok {
			props, _ := raw.(map[string]any)
			for name, sub := range props {
				value, present := m[name]
				if !present {
					continue
				}
				v.apply(sub, value, append(path, name))
			}
		}
	}
	if raw, ok := obj["required"]; ok {
		if m, ok := instance.(map[string]any); ok {
			arr, _ := raw.([]any)
			for _, item := range arr {
				name, _ := item.(string)
				if _, present := m[name]; !present {
					v.add(path, "required", fmt.Sprintf("missing required property %q", name))
				}
			}
		}
	}
	if raw, ok := obj["additionalProperties"]; ok {
		if allowed, ok := raw.(bool); ok && !allowed {
			if m, ok := instance.(map[string]any); ok {
				props, _ := obj["properties"].(map[string]any)
				for name := range m {
					if _, known := props[name]; !known {
						v.add(append(path, name), "additionalProperties", fmt.Sprintf("additional property %q is not allowed", name))
					}
				}
			}
		}
	}
	if raw, ok := obj["allOf"]; ok {
		arr, _ := raw.([]any)
		for _, sub := range arr {
			v.apply(sub, instance, path)
		}
	}
	if rawIf, ok := obj["if"]; ok {
		probe := validator{label: v.label}
		probe.apply(rawIf, instance, path)
		if len(probe.failures) == 0 {
			if rawThen, ok := obj["then"]; ok {
				v.apply(rawThen, instance, path)
			}
		} else if rawElse, ok := obj["else"]; ok {
			v.apply(rawElse, instance, path)
		}
	}
	if raw, ok := obj["not"]; ok {
		probe := validator{label: v.label}
		probe.apply(raw, instance, path)
		if len(probe.failures) == 0 {
			v.add(path, "not", "value matches the not schema")
		}
	}
}

func matchesType(typ string, instance any) bool {
	switch typ {
	case "object":
		_, ok := instance.(map[string]any)
		return ok
	case "array":
		_, ok := instance.([]any)
		return ok
	case "string":
		_, ok := instance.(string)
		return ok
	case "boolean":
		_, ok := instance.(bool)
		return ok
	case "integer":
		return isInteger(instance)
	default:
		return false
	}
}

func typeName(instance any) string {
	if instance == nil {
		return "null"
	}
	switch instance.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64, json.Number, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		if isInteger(instance) {
			return "integer"
		}
		return "number"
	default:
		return fmt.Sprintf("%T", instance)
	}
}

func isInteger(instance any) bool {
	if _, ok := instance.(bool); ok {
		return false
	}
	switch n := instance.(type) {
	case float64:
		return !math.IsNaN(n) && !math.IsInf(n, 0) && math.Trunc(n) == n
	case json.Number:
		f, err := n.Float64()
		return err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) && math.Trunc(f) == f
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func asFloat(instance any) (float64, bool) {
	if _, ok := instance.(bool); ok {
		return 0, false
	}
	switch n := instance.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}

func asNonNegativeInt(instance any) (int, bool) {
	f, ok := asFloat(instance)
	if !ok || f < 0 || math.Trunc(f) != f {
		return 0, false
	}
	return int(f), true
}

func jsonEqual(a, b any) bool {
	a = normalizeJSON(a)
	b = normalizeJSON(b)
	switch av := a.(type) {
	case nil:
		return b == nil
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for key, value := range av {
			other, present := bv[key]
			if !present || !jsonEqual(value, other) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func normalizeJSON(v any) any {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return n.String()
		}
		return f
	case int:
		return float64(n)
	case int8:
		return float64(n)
	case int16:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint8:
		return float64(n)
	case uint16:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	case []any:
		out := make([]any, len(n))
		for i, item := range n {
			out[i] = normalizeJSON(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(n))
		for key, value := range n {
			out[key] = normalizeJSON(value)
		}
		return out
	default:
		return v
	}
}
