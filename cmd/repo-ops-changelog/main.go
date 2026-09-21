package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/haesol-shin/.github/internal/changelog"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("repo-ops-changelog", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "changelog.d", "changelog fragment directory")
	changelogPath := flags.String("changelog", "CHANGELOG.md", "changelog file")
	version := flags.String("version", "", "release version (vX.Y.Z)")
	date := flags.String("date", "", "release date (YYYY-MM-DD)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || *version == "" || *date == "" {
		fmt.Fprintln(stderr, "usage: repo-ops-changelog [--root PATH] [--changelog PATH] --version vX.Y.Z --date YYYY-MM-DD")
		return 2
	}
	consumed, err := changelog.Fold(*root, *changelogPath, *version, *date)
	if err != nil {
		fmt.Fprintf(stderr, "repo-ops-changelog: error: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "folded %s from %d fragment(s)\n", *version, len(consumed))
	for _, path := range consumed {
		fmt.Fprintln(stdout, path)
	}
	return 0
}
