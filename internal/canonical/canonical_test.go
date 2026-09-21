package canonical

import "testing"

const (
	legacyOutcome = "Add a trusted validator without executing pull request code."
	legacyDigest  = "sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871"
	v1Digest      = "sha256:0dbcd01f3a688d6e6c688048de2f13c525e3f6df968573ae7f5c4fbbdeae59dc"
)

func TestNormalizeTextCRLFAndTrailingWhitespace(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"crlf", "hello\r\nworld\r\n", "hello\nworld\n"},
		{"cr", "hello\rworld\r", "hello\nworld\n"},
		{"trailing spaces", "hello  \t  ", "hello\n"},
		{"empty", "", "\n"},
		{"already lf", "hello\n", "hello\n"},
		{"mixed trailing newlines", "hello\n\n", "hello\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeText(tc.in); got != tc.want {
				t.Fatalf("NormalizeText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCanonicalDigestLegacyKnownAnswer(t *testing.T) {
	t.Parallel()
	got, err := CanonicalDigest(map[string]any{
		"outcome": NormalizeText(legacyOutcome),
		"risk":    "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != legacyDigest {
		t.Fatalf("legacy digest = %s, want %s", got, legacyDigest)
	}
}

func TestCanonicalDigestV1KnownAnswer(t *testing.T) {
	t.Parallel()
	got, err := CanonicalDigest(map[string]any{
		"issue":   42,
		"outcome": NormalizeText(legacyOutcome),
		"risk":    "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != v1Digest {
		t.Fatalf("v1 digest = %s, want %s", got, v1Digest)
	}
}

func TestCanonicalDigestKeySortAndNestedMaps(t *testing.T) {
	t.Parallel()
	left, err := CanonicalDigest(map[string]any{"b": 1, "a": 2})
	if err != nil {
		t.Fatal(err)
	}
	right, err := CanonicalDigest(map[string]any{"a": 2, "b": 1})
	if err != nil {
		t.Fatal(err)
	}
	if left != right {
		t.Fatalf("key order changed digest: %s vs %s", left, right)
	}
	if left != "sha256:d3626ac30a87e6f7a6428233b3c68299976865fa5508e4267c5415c76af7a772" {
		t.Fatalf("sorted keys digest = %s", left)
	}

	nested, err := CanonicalDigest(map[string]any{
		"z": map[string]any{"b": 1, "a": 2},
		"a": []any{3, 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if nested != "sha256:00fd0c489d898f44a7e856291d814b475b051111b42a197167de3af91337bd27" {
		t.Fatalf("nested digest = %s", nested)
	}
}

func TestCanonicalDigestUTF8AndUnescapedASCII(t *testing.T) {
	t.Parallel()
	unicode, err := CanonicalDigest(map[string]any{"t": "em—dash"})
	if err != nil {
		t.Fatal(err)
	}
	if unicode != "sha256:cf3bd0e7e1a9a870cb8b042e083d7c10bca48ff2b150d02ab4c0730c44f365a9" {
		t.Fatalf("unicode digest = %s", unicode)
	}
	amp, err := CanonicalDigest(map[string]any{"t": "a&b<c>"})
	if err != nil {
		t.Fatal(err)
	}
	if amp != "sha256:1a286c3e9404bed031b0ffc963b3a81d109c329272910e5c3a5fb6241be24b1d" {
		t.Fatalf("amp digest = %s", amp)
	}
}
