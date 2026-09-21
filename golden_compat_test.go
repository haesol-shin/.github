package repositorypolicy_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/haesol-shin/.github/internal/canonical"
	"github.com/haesol-shin/.github/internal/schema"
)

type digestGolden struct {
	Vectors []struct {
		ID      string `json:"id"`
		Kind    string `json:"kind"`
		Payload any    `json:"payload"`
		Text    string `json:"text"`
		Digest  string `json:"digest"`
	} `json:"vectors"`
}

type failureIdentity struct {
	InstancePath string `json:"instance_path"`
	Keyword      string `json:"keyword"`
}

type schemaGolden struct {
	Cases []struct {
		Schema            string            `json:"schema"`
		SchemaID          string            `json:"schema_id"`
		Label             string            `json:"label"`
		EmptyObject       []failureIdentity `json:"empty_object"`
		RequiredOmissions []struct {
			Omitted  string            `json:"omitted"`
			Failures []failureIdentity `json:"failures"`
		} `json:"required_omissions"`
	} `json:"cases"`
}

func TestFrozenDigestGoldens(t *testing.T) {
	var golden digestGolden
	decodeJSONFile(t, "tests/testdata/go-oracle/digests.json", &golden, true)
	if len(golden.Vectors) == 0 {
		t.Fatal("digest golden has no vectors")
	}
	for _, vector := range golden.Vectors {
		t.Run(vector.ID, func(t *testing.T) {
			var got string
			var err error
			switch vector.Kind {
			case "canonical":
				got, err = canonical.CanonicalDigest(vector.Payload)
			case "plan":
				got, err = canonical.PlanDigest(vector.Text)
			default:
				t.Fatalf("unknown digest vector kind %q", vector.Kind)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != vector.Digest {
				t.Fatalf("digest mismatch: got %s, want %s", got, vector.Digest)
			}
		})
	}
}

func TestFrozenSchemaFailureIdentities(t *testing.T) {
	var golden schemaGolden
	decodeJSONFile(t, "tests/testdata/go-oracle/schema-failures.json", &golden, false)
	samples := schemaSamples()
	if len(golden.Cases) != len(samples) {
		t.Fatalf("schema golden contains %d cases, want %d", len(golden.Cases), len(samples))
	}
	for _, testCase := range golden.Cases {
		t.Run(testCase.Schema, func(t *testing.T) {
			sample, ok := samples[testCase.Schema]
			if !ok {
				t.Fatalf("no valid sample for %s", testCase.Schema)
			}
			schemaPath := filepath.Join("contracts", "v0.1.0", testCase.Schema)
			var schemaDocument map[string]any
			decodeJSONFile(t, schemaPath, &schemaDocument, false)
			if schemaDocument["$id"] != testCase.SchemaID {
				t.Fatalf("schema ID mismatch: got %v, want %s", schemaDocument["$id"], testCase.SchemaID)
			}

			assertSchemaFailures(t, map[string]any{}, schemaPath, testCase.Label, testCase.EmptyObject)
			for _, omission := range testCase.RequiredOmissions {
				instance := cloneObject(t, sample)
				delete(instance, omission.Omitted)
				assertSchemaFailures(t, instance, schemaPath, testCase.Label, omission.Failures)
			}
		})
	}
}

func assertSchemaFailures(t *testing.T, instance map[string]any, schemaPath, label string, want []failureIdentity) {
	t.Helper()
	failures, err := schema.ValidateFile(instance, schemaPath, label)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]failureIdentity, 0, len(failures))
	for _, failure := range failures {
		if failure.Label != label {
			t.Fatalf("failure label mismatch: got %q, want %q", failure.Label, label)
		}
		got = append(got, failureIdentity{InstancePath: failure.InstancePath, Keyword: failure.Keyword})
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("failure identities mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func schemaSamples() map[string]map[string]any {
	digest := "sha256:" + strings.Repeat("a", 64)
	commit := strings.Repeat("a", 40)
	return map[string]map[string]any{
		"changelog-declaration.schema.json": {
			"record": "repo-ops.changelog.v1", "kind": "not-required", "value": "receipt-validation",
		},
		"intent.schema.json": {
			"record": "repo-ops.intent.v1", "decision": "accepted", "risk": "low", "intent": digest, "supersedes": "none",
		},
		"merge-review.schema.json": {
			"record": "repo-ops.merge-review.v1", "verdict": "merge-ready", "risk": "low", "intent": "none", "plan": "none",
			"base": commit, "head": commit, "diff": digest, "runtime": "x", "model": "x",
		},
		"plan-approval.schema.json": {
			"record": "repo-ops.plan-approval.v1", "decision": "approved", "risk": "high", "issue": float64(1),
			"intent": digest, "plan": digest, "plan-commit": commit,
		},
		"repository-policy.schema.json": {
			"contract": "repo-ops/v0.1.0", "profile": "python-engine",
			"quality":   map[string]any{"check": "quality", "commands": []any{"test"}},
			"review":    map[string]any{"shared_identity": false},
			"changelog": map[string]any{"mode": "none"},
			"release":   map[string]any{"enabled": false},
		},
	}
}

func cloneObject(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(encoded, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func decodeJSONFile(t *testing.T, path string, target any, useNumber bool) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	if useNumber {
		decoder.UseNumber()
	}
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
}
