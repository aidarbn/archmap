// Command archmap builds an interactive, function-level architecture map of a
// repository as one offline HTML file.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/aidarbn/archmap/internal/extract"
	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/lang/all"
	"github.com/aidarbn/archmap/internal/render"
)

// version is set with -ldflags "-X main.version=…"; falls back to build info.
var version = "dev"

const usage = `archmap - interactive function-level architecture map

Usage:
  archmap extract [--root DIR] [--base REF] [--tests] --out facts.json
  archmap render  --facts facts.json [--model arch.model.json] --out arch.html
  archmap map     [--root DIR] [--base REF] [--tests] [--model FILE] [--out FILE] [--open]
  archmap serve   (not implemented)
  archmap version
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "extract":
		err = cmdExtract(args[1:], stdout, stderr)
	case "render":
		err = cmdRender(args[1:], stdout, stderr)
	case "map":
		err = cmdMap(args[1:], stdout, stderr)
	case "serve":
		fmt.Fprintln(stderr, "archmap serve: not implemented")
		return 2
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, "archmap", buildVersion())
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "archmap: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		return 0
	default:
		fmt.Fprintln(stderr, "archmap:", err)
		return 1
	}
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

func newFlags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("archmap "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func extractFacts(root, base string, tests bool) (*facts.Facts, error) {
	return extract.Run(extract.Options{Root: root, Base: base, WithTests: tests, Parsers: all.Parsers()})
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := marshal(v, " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// marshal encodes like Python's json.dumps(ensure_ascii=False): no HTML escaping.
func marshal(v any, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", indent)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func cmdExtract(args []string, stdout, stderr io.Writer) error {
	fs := newFlags("extract", stderr)
	root := fs.String("root", ".", "repository root")
	base := fs.String("base", "", "git ref to diff against (default: merge-base with the default branch)")
	tests := fs.Bool("tests", false, "include test files")
	out := fs.String("out", "", "output facts.json (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("--out is required")
	}
	f, err := extractFacts(*root, *base, *tests)
	if err != nil {
		return err
	}
	if err := writeJSON(*out, f); err != nil {
		return err
	}
	fmt.Fprintln(stdout, extract.Summary(f))
	return nil
}

func cmdRender(args []string, stdout, stderr io.Writer) error {
	fs := newFlags("render", stderr)
	factsPath := fs.String("facts", "", "facts.json (required)")
	model := fs.String("model", "", "arch.model.json (optional)")
	out := fs.String("out", "", "output HTML (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *factsPath == "" || *out == "" {
		return errors.New("--facts and --out are required")
	}
	if err := render.Files(*factsPath, *model, *out); err != nil {
		return err
	}
	fmt.Fprintln(stdout, *out)
	return nil
}

func cmdMap(args []string, stdout, stderr io.Writer) error {
	fs := newFlags("map", stderr)
	root := fs.String("root", ".", "repository root")
	base := fs.String("base", "", "git ref to diff against (default: merge-base with the default branch)")
	tests := fs.Bool("tests", false, "include test files")
	model := fs.String("model", "", "arch.model.json (default: <root>/arch.model.json if present)")
	out := fs.String("out", "", "output HTML (default: <root>/.archmap/arch.html)")
	open := fs.Bool("open", false, "open the result in the default browser")
	if err := fs.Parse(args); err != nil {
		return err
	}
	f, err := extractFacts(*root, *base, *tests)
	if err != nil {
		return err
	}
	if *out == "" {
		*out = filepath.Join(*root, ".archmap", "arch.html")
	}
	if *model == "" {
		if p := filepath.Join(*root, "arch.model.json"); fileExists(p) {
			*model = p
		}
	}
	fb, err := marshal(f, "")
	if err != nil {
		return err
	}
	var mb []byte
	if *model != "" {
		if mb, err = os.ReadFile(*model); err != nil {
			return err
		}
	}
	html, err := render.HTML(fb, mb)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*out, html, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(stdout, extract.Summary(f))
	fmt.Fprintln(stdout, "\n"+*out)
	if *open {
		return openBrowser(*out)
	}
	return nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func openBrowser(p string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", p)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", p)
	default:
		cmd = exec.Command("xdg-open", p)
	}
	return cmd.Start()
}
