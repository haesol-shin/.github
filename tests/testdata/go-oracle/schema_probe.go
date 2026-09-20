package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/haesol-shin/.github/internal/schema"
)

type probeRow struct {
	ID       string `json:"id"`
	Schema   string `json:"schema"`
	Label    string `json:"label"`
	Instance any    `json:"instance"`
}

type probeInput struct {
	Rows []probeRow `json:"rows"`
}

type identity struct {
	InstancePath string `json:"instance_path"`
	Keyword      string `json:"keyword"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: schema_probe input.json")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var input probeInput
	if err := json.Unmarshal(raw, &input); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	for _, row := range input.Rows {
		failures, err := schema.ValidateFile(row.Instance, row.Schema, row.Label)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		identities := make([]identity, 0, len(failures))
		for _, failure := range failures {
			identities = append(identities, identity{
				InstancePath: failure.InstancePath,
				Keyword:      failure.Keyword,
			})
		}
		if err := encoder.Encode(map[string]any{
			"id":       row.ID,
			"failures": identities,
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
