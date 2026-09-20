package canonical

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// NormalizeText matches validate.py::normalize_text: CRLF/CR to LF, rstrip, one trailing LF.
func NormalizeText(value string) string {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.TrimRightFunc(normalized, pythonSpace)
	return normalized + "\n"
}

// CanonicalDigest matches validate.py::canonical_digest: sha256 of compact
// lexicographically key-sorted UTF-8 JSON (ensure_ascii=False, separators (',', ':')).
func CanonicalDigest(payload any) (string, error) {
	encoded, err := marshalCanonical(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// PlanDigest matches validate.py::plan_digest.
func PlanDigest(content string) (string, error) {
	return CanonicalDigest(map[string]any{"text": NormalizeText(content)})
}

func pythonSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', '\x1c', '\x1d', '\x1e', '\x1f', '\u0085', '\u00a0':
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

func marshalCanonical(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeJSON(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
		return nil
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		return nil
	case int:
		buf.WriteString(strconv.FormatInt(int64(t), 10))
		return nil
	case int8:
		buf.WriteString(strconv.FormatInt(int64(t), 10))
		return nil
	case int16:
		buf.WriteString(strconv.FormatInt(int64(t), 10))
		return nil
	case int32:
		buf.WriteString(strconv.FormatInt(int64(t), 10))
		return nil
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
		return nil
	case uint:
		buf.WriteString(strconv.FormatUint(uint64(t), 10))
		return nil
	case uint8:
		buf.WriteString(strconv.FormatUint(uint64(t), 10))
		return nil
	case uint16:
		buf.WriteString(strconv.FormatUint(uint64(t), 10))
		return nil
	case uint32:
		buf.WriteString(strconv.FormatUint(uint64(t), 10))
		return nil
	case uint64:
		buf.WriteString(strconv.FormatUint(t, 10))
		return nil
	case float32:
		return writeFloat(buf, float64(t))
	case float64:
		return writeFloat(buf, t)
	case json.Number:
		if t == "" {
			return fmt.Errorf("invalid json.Number")
		}
		buf.WriteString(string(t))
		return nil
	case string:
		writeString(buf, t)
		return nil
	case map[string]any:
		return writeObject(buf, t)
	case []any:
		return writeArray(buf, t)
	default:
		return fmt.Errorf("unsupported canonical type %T", v)
	}
}

func writeFloat(buf *bytes.Buffer, f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return fmt.Errorf("unsupported float value")
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	buf.WriteString(s)
	return nil
}

func writeString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for i := range len(s) {
		c := s[i]
		switch c {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if c < 0x20 {
				buf.WriteString(`\u00`)
				buf.WriteByte("0123456789abcdef"[c>>4])
				buf.WriteByte("0123456789abcdef"[c&0x0f])
			} else {
				buf.WriteByte(c)
			}
		}
	}
	buf.WriteByte('"')
}

func writeObject(buf *bytes.Buffer, m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		writeString(buf, k)
		buf.WriteByte(':')
		if err := writeJSON(buf, m[k]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

func writeArray(buf *bytes.Buffer, items []any) error {
	buf.WriteByte('[')
	for i, item := range items {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeJSON(buf, item); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}
