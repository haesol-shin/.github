package fixture

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testdata(name string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller")
	}
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

func repoFixture(name string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "fixtures", name)
}

func TestDecodeJSONRejectsTrailingData(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{} {}`, `{} trailing`} {
		if _, err := decodeJSON([]byte(raw)); err == nil {
			t.Fatalf("decodeJSON(%q) accepted trailing data", raw)
		}
	}
}

func TestLoadNestedMapsAndArrays(t *testing.T) {
	t.Parallel()
	state, expected, err := Load(testdata("nested.json"))
	if err != nil {
		t.Fatal(err)
	}
	if expected == nil || *expected != "nested-ok" {
		t.Fatalf("expected = %v, want nested-ok", expected)
	}
	nested := state["nested"].(map[string]any)
	b := nested["b"].(map[string]any)
	if b["c"] != int64(3) {
		t.Fatalf("nested.b.c = %v (%T)", b["c"], b["c"])
	}
	if nested["a"] != int64(1) {
		t.Fatalf("nested.a = %v", nested["a"])
	}
	items := state["items"].([]any)
	first := items[0].(map[string]any)
	if first["v"] != "z" {
		t.Fatalf("items.0.v = %v", first["v"])
	}
	if first["id"] != int64(1) {
		t.Fatalf("items.0.id = %v", first["id"])
	}
	second := items[1].(map[string]any)
	if second["v"] != "y" {
		t.Fatalf("items.1.v = %v", second["v"])
	}

	base, _, err := Load(testdata("base.json"))
	if err != nil {
		t.Fatal(err)
	}
	baseNested := base["nested"].(map[string]any)
	baseB := baseNested["b"].(map[string]any)
	if baseB["c"] != int64(2) {
		t.Fatalf("deepcopy leaked into parent nested.b.c = %v", baseB["c"])
	}
}

func TestLoadWholeArrayReplacement(t *testing.T) {
	t.Parallel()
	state, expected, err := Load(testdata("whole-array.json"))
	if err != nil {
		t.Fatal(err)
	}
	if expected != nil {
		t.Fatalf("expected = %v, want nil", expected)
	}
	tags := state["tags"].([]any)
	if len(tags) != 2 || tags[0] != "new" || tags[1] != "also" {
		t.Fatalf("tags = %#v", tags)
	}

	base, _, err := Load(testdata("base.json"))
	if err != nil {
		t.Fatal(err)
	}
	baseTags := base["tags"].([]any)
	if len(baseTags) != 1 || baseTags[0] != "old" {
		t.Fatalf("deepcopy leaked into parent tags = %#v", baseTags)
	}
}

func TestLoadInvalidTraversal(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"invalid-missing.json", "invalid-index.json", "invalid-scalar.json"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, _, err := Load(testdata(name))
			if err == nil {
				t.Fatal("want traversal error")
			}
		})
	}
}

func TestLoadRealFixturesExpectedAndOverlay(t *testing.T) {
	t.Parallel()

	stale, expected, err := Load(repoFixture("invalid-stale-head.json"))
	if err != nil {
		t.Fatal(err)
	}
	if expected == nil || *expected != "merge-review head does not match the current pull request" {
		t.Fatalf("stale expected = %v", expected)
	}
	comments := stale["comments"].([]any)
	body := comments[0].(map[string]any)["body"].(string)
	if !strings.Contains(body, "head:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee") {
		t.Fatalf("stale body was not replaced: %s", body)
	}

	related, expected, err := Load(repoFixture("valid-v1-related.json"))
	if err != nil {
		t.Fatal(err)
	}
	if expected != nil {
		t.Fatalf("v1 expected = %v, want nil", expected)
	}
	intents := related["intent_comments"].([]any)
	if len(intents) != 2 {
		t.Fatalf("intent_comments length = %d", len(intents))
	}
	prBody := related["pull_request"].(map[string]any)["body"].(string)
	if !strings.Contains(prBody, "Related #42") {
		t.Fatalf("pull_request.body overlay missing Related #42")
	}

	fragment, expected, err := Load(repoFixture("valid-required-fragment.json"))
	if err != nil {
		t.Fatal(err)
	}
	if expected != nil {
		t.Fatalf("fragment expected = %v, want nil", expected)
	}
	files := fragment["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("files = %#v", files)
	}
	filename := files[0].(map[string]any)["filename"].(string)
	if filename != "changelog.d/42-validator.md" {
		t.Fatalf("filename = %s", filename)
	}
}
