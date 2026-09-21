package collect

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDecodePolicyMatchesRepoPolicy(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "repo-policy.yml"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodePolicy(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"contract": "repo-ops/v0.1.0",
		"profile":  "operations",
		"quality": map[string]any{
			"check":    "quality",
			"commands": []any{"python -m unittest discover -s tests -v"},
		},
		"review": map[string]any{
			"shared_identity": true,
		},
		"changelog": map[string]any{
			"mode": "fragments",
			"root": "changelog.d",
		},
		"release": map[string]any{
			"enabled": true,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("policy object mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestDecodePolicyRejectsInvalidYAML(t *testing.T) {
	_, err := DecodePolicy(":\n  -")
	if err == nil {
		t.Fatal("expected YAML error")
	}
}

func TestDecodePolicyEmptyIsNil(t *testing.T) {
	got, err := DecodePolicy("  \n")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("got %#v, want nil", got)
	}
}
