package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/aidarbn/archmap/internal/lint"
)

// cmdLint checks the `arch:` scenario markup; exit code 1 when it has problems.
func cmdLint(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("lint", stderr)
	root := fs.String("root", ".", "repository root")
	format := fs.String("format", "text", "output format: text | json")
	tests := fs.Bool("tests", false, "include test files")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(stderr, "archmap: --format must be text or json")
		return 2
	}
	f, err := extractFacts(*root, "", *tests, false)
	if err != nil {
		fmt.Fprintln(stderr, "archmap:", err)
		return 1
	}
	res := lint.Run(*root, f)
	if *format == "json" {
		b, err := marshal(res, "  ")
		if err != nil {
			fmt.Fprintln(stderr, "archmap:", err)
			return 1
		}
		fmt.Fprintln(stdout, string(b))
	} else {
		for _, p := range res.Problems {
			fmt.Fprintln(stdout, p)
		}
		if len(res.Problems) == 0 {
			fmt.Fprintf(stdout, "archmap lint: ok (%d flows, %d annotated functions)\n", res.Flows, res.Annotated)
		}
	}
	if len(res.Problems) > 0 {
		fmt.Fprintf(stderr, "archmap: %d problems in arch: markup\n", len(res.Problems))
		return 1
	}
	return 0
}
