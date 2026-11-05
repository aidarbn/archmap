package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/aidarbn/archmap/internal/lint"
)

// cmdLint checks the `arch:` scenario markup and the arch/*.arch specification
// against the code; exit code 1 when it has problems (with --strict, warnings too).
func cmdLint(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("lint", stderr)
	root := fs.String("root", ".", "repository root")
	format := fs.String("format", "text", "output format: text | json")
	tests := fs.Bool("tests", false, "include test files")
	strict := fs.Bool("strict", false, "count warnings as errors (for CI)")
	missing := fs.Bool("missing-docs", false, "list the modules, entry points, endpoints, actors, externals and scenarios (arch:flow) without a description")
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
	docs := lint.Docs(f)
	if !*missing {
		docs.Missing = nil
	}
	res.Docs = &docs
	failed := len(res.Problems)
	if *strict {
		failed += len(res.Warnings)
	}
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
		for _, p := range res.Warnings {
			if *strict {
				fmt.Fprintln(stdout, p)
			} else {
				fmt.Fprintln(stdout, "warning:", p)
			}
		}
		fmt.Fprintln(stdout, "info: "+docs.Line())
		for _, m := range docs.Missing {
			fmt.Fprintln(stdout, "info:", m)
		}
		if res.ClientLinks > 0 {
			fmt.Fprintf(stdout, "info: %d journey links are client calls (no link in the code; --format json lists them)\n", res.ClientLinks)
		}
		if res.Planned > 0 {
			fmt.Fprintf(stdout, "info: %d planned steps in the spec (no code yet)\n", res.Planned)
		}
		if failed == 0 {
			msg := fmt.Sprintf("archmap lint: ok (%d flows, %d annotated functions", res.Flows, res.Annotated)
			if res.Spec {
				msg += fmt.Sprintf(", %d spec flows", res.SpecFlows)
			}
			fmt.Fprintln(stdout, msg+")")
		}
	}
	if failed > 0 {
		what := "problems in arch: markup"
		if *strict && len(res.Warnings) > 0 {
			what = fmt.Sprintf("problems in arch: markup and spec (%d warnings counted by --strict)", len(res.Warnings))
		}
		fmt.Fprintf(stderr, "archmap: %d %s\n", failed, what)
		return 1
	}
	return 0
}
