package evaluate

import (
	"encoding/json"
	"math"
	"strings"
)

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asSlice(v any) []any {
	switch s := v.(type) {
	case []any:
		return s
	case nil:
		return nil
	default:
		return nil
	}
}

func asEntries(v any) []map[string]any {
	items := asSlice(v)
	if items == nil {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m := asMap(item); m != nil {
			out = append(out, m)
		}
	}
	return out
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		if int64(int(n)) != n {
			return 0, false
		}
		return int(n), true
	case float64:
		if math.Trunc(n) != n || n > float64(math.MaxInt) || n < float64(math.MinInt) {
			return 0, false
		}
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		if int64(int(i)) != i {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func asBool(v any) (bool, bool) {
	b, ok := v.(bool)
	return b, ok
}

func nested(m map[string]any, keys ...string) any {
	var cur any = m
	for _, key := range keys {
		obj := asMap(cur)
		if obj == nil {
			return nil
		}
		cur = obj[key]
	}
	return cur
}

func sameValue(a, b any) bool {
	if ai, ok := asInt(a); ok {
		if bi, ok := asInt(b); ok {
			return ai == bi
		}
	}
	if as, ok := a.(string); ok {
		if bs, ok := b.(string); ok {
			return as == bs
		}
	}
	return a == b
}

func stringList(v any) []string {
	items := asSlice(v)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func inSet(value string, set []string) bool {
	return containsString(set, value)
}

func copyEntry(entry map[string]any, surface string) map[string]any {
	out := make(map[string]any, len(entry)+1)
	for k, v := range entry {
		out[k] = v
	}
	out["surface"] = surface
	return out
}

func orEmptyString(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return ""
	}
	return s
}

func isASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func normalizeSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}
