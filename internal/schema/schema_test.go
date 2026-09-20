package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateFileValidContractInstances(t *testing.T) {
	t.Parallel()
	cases := []struct {
		schema   string
		label    string
		instance map[string]any
	}{
		{
			schema: "intent.schema.json",
			label:  "repo-ops.intent.v1",
			instance: map[string]any{
				"record":     "repo-ops.intent.v1",
				"decision":   "accepted",
				"risk":       "high",
				"intent":     "sha256:" + strings.Repeat("a", 64),
				"supersedes": "none",
			},
		},
		{
			schema: "plan-approval.schema.json",
			label:  "plan approval",
			instance: map[string]any{
				"record":      "repo-ops.plan-approval.v1",
				"decision":    "approved",
				"risk":        "high",
				"issue":       float64(5),
				"intent":      "sha256:" + strings.Repeat("b", 64),
				"plan":        "sha256:" + strings.Repeat("c", 64),
				"plan-commit": strings.Repeat("d", 40),
			},
		},
		{
			schema: "merge-review.schema.json",
			label:  "merge-review receipt",
			instance: map[string]any{
				"record":  "repo-ops.merge-review.v1",
				"verdict": "merge-ready",
				"risk":    "low",
				"intent":  "none",
				"plan":    "none",
				"base":    strings.Repeat("e", 40),
				"head":    strings.Repeat("f", 40),
				"diff":    "sha256:" + strings.Repeat("0", 64),
				"runtime": "go1.22",
				"model":   "local",
			},
		},
		{
			schema: "repository-policy.schema.json",
			label:  "repository policy",
			instance: map[string]any{
				"contract": "repo-ops/v0.1.0",
				"profile":  "operations",
				"quality": map[string]any{
					"check":    "quality",
					"commands": []any{"go test ./..."},
				},
				"review": map[string]any{"shared_identity": true},
				"changelog": map[string]any{
					"mode": "fragments",
					"root": "changelog.d",
				},
				"release": map[string]any{"enabled": true},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.schema, func(t *testing.T) {
			t.Parallel()
			failures, err := ValidateFile(tc.instance, contractSchema(tc.schema), tc.label)
			if err != nil {
				t.Fatalf("ValidateFile: %v", err)
			}
			if len(failures) != 0 {
				t.Fatalf("unexpected failures: %+v", failures)
			}
		})
	}
}

func TestValidateFileEmptyObjectRequired(t *testing.T) {
	t.Parallel()
	failures, err := ValidateFile(map[string]any{}, contractSchema("intent.schema.json"), "repo-ops.intent.v1")
	if err != nil {
		t.Fatalf("ValidateFile: %v", err)
	}
	if len(failures) == 0 {
		t.Fatal("expected required failures")
	}
	seen := map[string]bool{}
	prevPath, prevKeyword, prevMessage := "", "", ""
	for i, f := range failures {
		if f.Label != "repo-ops.intent.v1" {
			t.Errorf("failure %d label = %q", i, f.Label)
		}
		if f.Keyword == "required" {
			seen[f.Message] = true
		}
		if f.InstancePath > prevPath {
			prevPath, prevKeyword, prevMessage = f.InstancePath, f.Keyword, f.Message
			continue
		}
		if f.InstancePath == prevPath && f.Keyword > prevKeyword {
			prevKeyword, prevMessage = f.Keyword, f.Message
			continue
		}
		if f.InstancePath == prevPath && f.Keyword == prevKeyword && f.Message >= prevMessage {
			prevMessage = f.Message
			continue
		}
		if i > 0 {
			t.Fatalf("failures not sorted at %d: %+v then %+v", i, failures[i-1], f)
		}
		prevPath, prevKeyword, prevMessage = f.InstancePath, f.Keyword, f.Message
	}
	for _, field := range []string{"record", "decision", "risk", "intent", "supersedes"} {
		want := `missing required property "` + field + `"`
		if !seen[want] {
			t.Errorf("missing required failure for %s in %+v", field, failures)
		}
	}
}

func TestValidateFileKeywords(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeSchema(t, dir, "object.json", `{
		"type": "object",
		"additionalProperties": false,
		"required": ["name"],
		"properties": {
			"name": {"type": "string", "minLength": 2, "pattern": "^[a-z]+$"},
			"kind": {"const": "fixed"},
			"level": {"enum": ["low", "high"]},
			"count": {"type": "integer", "minimum": 1},
			"flag": {"type": "boolean"},
			"tags": {"type": "array", "minItems": 1, "items": {"type": "string", "minLength": 1}}
		}
	}`)
	writeSchema(t, dir, "ifthen.json", `{
		"type": "object",
		"properties": {
			"mode": {"enum": ["fragments", "none"]},
			"root": {"type": "string", "minLength": 1}
		},
		"allOf": [{
			"if": {"properties": {"mode": {"const": "fragments"}}, "required": ["mode"]},
			"then": {"required": ["root"]}
		}]
	}`)
	writeSchema(t, dir, "not.json", `{"not":{"const":"replace-me"}}`)

	t.Run("additionalProperties", func(t *testing.T) {
		t.Parallel()
		failures := mustValidate(t, map[string]any{"name": "ok", "extra": true}, filepath.Join(dir, "object.json"), "obj")
		assertFailure(t, failures, Failure{Label: "obj", InstancePath: "/extra", Keyword: "additionalProperties"})
	})
	t.Run("const", func(t *testing.T) {
		t.Parallel()
		failures := mustValidate(t, map[string]any{"name": "ok", "kind": "other"}, filepath.Join(dir, "object.json"), "obj")
		assertFailure(t, failures, Failure{Label: "obj", InstancePath: "/kind", Keyword: "const"})
	})
	t.Run("enum", func(t *testing.T) {
		t.Parallel()
		failures := mustValidate(t, map[string]any{"name": "ok", "level": "medium"}, filepath.Join(dir, "object.json"), "obj")
		assertFailure(t, failures, Failure{Label: "obj", InstancePath: "/level", Keyword: "enum"})
	})
	t.Run("patternAndMinLength", func(t *testing.T) {
		t.Parallel()
		failures := mustValidate(t, map[string]any{"name": "A"}, filepath.Join(dir, "object.json"), "obj")
		assertFailure(t, failures, Failure{Label: "obj", InstancePath: "/name", Keyword: "minLength"})
		assertFailure(t, failures, Failure{Label: "obj", InstancePath: "/name", Keyword: "pattern"})
	})
	t.Run("integerRejectsBoolean", func(t *testing.T) {
		t.Parallel()
		failures := mustValidate(t, map[string]any{"name": "ok", "count": true}, filepath.Join(dir, "object.json"), "obj")
		assertFailure(t, failures, Failure{Label: "obj", InstancePath: "/count", Keyword: "type"})
		for _, f := range failures {
			if f.Keyword == "minimum" {
				t.Fatalf("boolean must not be compared as a number: %+v", failures)
			}
		}
	})
	t.Run("minimum", func(t *testing.T) {
		t.Parallel()
		failures := mustValidate(t, map[string]any{"name": "ok", "count": float64(0)}, filepath.Join(dir, "object.json"), "obj")
		assertFailure(t, failures, Failure{Label: "obj", InstancePath: "/count", Keyword: "minimum"})
	})
	t.Run("boolean", func(t *testing.T) {
		t.Parallel()
		failures := mustValidate(t, map[string]any{"name": "ok", "flag": "yes"}, filepath.Join(dir, "object.json"), "obj")
		assertFailure(t, failures, Failure{Label: "obj", InstancePath: "/flag", Keyword: "type"})
	})
	t.Run("arrayMinItemsAndItems", func(t *testing.T) {
		t.Parallel()
		empty := mustValidate(t, map[string]any{"name": "ok", "tags": []any{}}, filepath.Join(dir, "object.json"), "obj")
		assertFailure(t, empty, Failure{Label: "obj", InstancePath: "/tags", Keyword: "minItems"})
		items := mustValidate(t, map[string]any{"name": "ok", "tags": []any{""}}, filepath.Join(dir, "object.json"), "obj")
		assertFailure(t, items, Failure{Label: "obj", InstancePath: "/tags/0", Keyword: "minLength"})
	})
	t.Run("ifThenRequired", func(t *testing.T) {
		t.Parallel()
		missing := mustValidate(t, map[string]any{"mode": "fragments"}, filepath.Join(dir, "ifthen.json"), "chg")
		assertFailure(t, missing, Failure{Label: "chg", InstancePath: "", Keyword: "required"})
		none := mustValidate(t, map[string]any{"mode": "none"}, filepath.Join(dir, "ifthen.json"), "chg")
		if len(none) != 0 {
			t.Fatalf("then must not apply: %+v", none)
		}
		ok := mustValidate(t, map[string]any{"mode": "fragments", "root": "changelog.d"}, filepath.Join(dir, "ifthen.json"), "chg")
		if len(ok) != 0 {
			t.Fatalf("valid fragments instance: %+v", ok)
		}
	})
	t.Run("nestedPolicyQuality", func(t *testing.T) {
		t.Parallel()
		instance := map[string]any{
			"contract": "repo-ops/v0.1.0",
			"profile":  "operations",
			"quality": map[string]any{
				"check":    "quality",
				"commands": []any{},
			},
			"review":    map[string]any{"shared_identity": true},
			"changelog": map[string]any{"mode": "none"},
			"release":   map[string]any{"enabled": false},
		}
		failures := mustValidate(t, instance, contractSchema("repository-policy.schema.json"), "repository policy")
		assertFailure(t, failures, Failure{Label: "repository policy", InstancePath: "/quality/commands", Keyword: "minItems"})
	})
	t.Run("changelogIfThenOnContractSchema", func(t *testing.T) {
		t.Parallel()
		instance := map[string]any{
			"contract":  "repo-ops/v0.1.0",
			"profile":   "operations",
			"quality":   map[string]any{"check": "quality", "commands": []any{"test"}},
			"review":    map[string]any{"shared_identity": false},
			"changelog": map[string]any{"mode": "fragments"},
			"release":   map[string]any{"enabled": true},
		}
		failures := mustValidate(t, instance, contractSchema("repository-policy.schema.json"), "repository policy")
		assertFailure(t, failures, Failure{Label: "repository policy", InstancePath: "/changelog", Keyword: "required"})
	})
	t.Run("not", func(t *testing.T) {
		t.Parallel()
		match := mustValidate(t, "replace-me", filepath.Join(dir, "not.json"), "n")
		assertFailure(t, match, Failure{Label: "n", InstancePath: "", Keyword: "not"})
		for _, f := range match {
			if f.Keyword != "not" {
				t.Fatalf("matching child must fail as keyword not, got %+v", match)
			}
		}
		miss := mustValidate(t, "plan-only", filepath.Join(dir, "not.json"), "n")
		if len(miss) != 0 {
			t.Fatalf("nonmatching child must pass: %+v", miss)
		}
	})
	t.Run("changelogNotReplaceMe", func(t *testing.T) {
		t.Parallel()
		path := contractSchema("changelog-declaration.schema.json")
		placeholder := mustValidate(t, map[string]any{
			"record": "repo-ops.changelog.v1",
			"kind":   "required",
			"value":  "replace-me",
		}, path, "changelog declaration")
		assertFailure(t, placeholder, Failure{Label: "changelog declaration", InstancePath: "/value", Keyword: "not"})
		for _, f := range placeholder {
			if f.InstancePath == "/value" && f.Keyword != "not" {
				t.Fatalf("replace-me must fail as keyword not, got %+v", placeholder)
			}
		}
		ok := mustValidate(t, map[string]any{
			"record": "repo-ops.changelog.v1",
			"kind":   "required",
			"value":  "fragment-added",
		}, path, "changelog declaration")
		if len(ok) != 0 {
			t.Fatalf("valid changelog declaration: %+v", ok)
		}
	})
}

func TestValidateFileUnknownKeywordFailsClosed(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "unknown.json")
	writeSchema(t, filepath.Dir(path), "unknown.json", `{"type":"object","oneOf":[]}`)
	_, err := ValidateFile(map[string]any{}, path, "x")
	if err == nil {
		t.Fatal("expected error for unsupported keyword")
	}
	if !strings.Contains(err.Error(), "oneOf") {
		t.Fatalf("error should name keyword: %v", err)
	}
}

func TestValidateFileMissingFile(t *testing.T) {
	t.Parallel()
	_, err := ValidateFile(map[string]any{}, filepath.Join(t.TempDir(), "missing.json"), "x")
	if err == nil {
		t.Fatal("expected read error")
	}
}

func TestIntegerAcceptsWholeJSONNumber(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSchema(t, dir, "n.json", `{"type":"integer","minimum":1}`)
	failures := mustValidate(t, json.Number("2"), filepath.Join(dir, "n.json"), "n")
	if len(failures) != 0 {
		t.Fatalf("json.Number integer should pass: %+v", failures)
	}
	failures = mustValidate(t, json.Number("1.5"), filepath.Join(dir, "n.json"), "n")
	assertFailure(t, failures, Failure{Label: "n", InstancePath: "", Keyword: "type"})
}

func writeSchema(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustValidate(t *testing.T, instance any, path, label string) []Failure {
	t.Helper()
	failures, err := ValidateFile(instance, path, label)
	if err != nil {
		t.Fatalf("ValidateFile: %v", err)
	}
	return failures
}

func assertFailure(t *testing.T, failures []Failure, want Failure) {
	t.Helper()
	for _, got := range failures {
		if got.Label == want.Label && got.InstancePath == want.InstancePath && got.Keyword == want.Keyword {
			if got.Message == "" {
				t.Fatalf("empty message for %+v", got)
			}
			return
		}
	}
	t.Fatalf("missing failure %+v in %+v", want, failures)
}

func contractSchema(name string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "contracts", "v0.1.0", name)
}
