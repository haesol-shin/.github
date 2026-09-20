package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/haesol-shin/.github/internal/canonical"
)

type vectorFile struct {
	Vectors []struct {
		ID      string `json:"id"`
		Kind    string `json:"kind"`
		Payload any    `json:"payload"`
		Text    string `json:"text"`
	} `json:"vectors"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: digest_probe digests.json\n")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var doc vectorFile
	if err := decoder.Decode(&doc); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	for _, vector := range doc.Vectors {
		var digest string
		switch vector.Kind {
		case "canonical":
			digest, err = canonical.CanonicalDigest(vector.Payload)
		case "plan":
			digest, err = canonical.PlanDigest(vector.Text)
		default:
			fmt.Fprintf(os.Stderr, "unknown kind %q\n", vector.Kind)
			os.Exit(1)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := enc.Encode(map[string]string{"id": vector.ID, "digest": digest}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
