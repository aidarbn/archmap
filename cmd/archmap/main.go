// Command archmap builds an interactive, function-level architecture map of a
// repository as one offline HTML file.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/aidarbn/archmap/internal/autosem"
	"github.com/aidarbn/archmap/internal/extract"
	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/lang/all"
	"github.com/aidarbn/archmap/internal/lint"
	"github.com/aidarbn/archmap/internal/render"
	"github.com/aidarbn/archmap/internal/serve"
	"github.com/aidarbn/archmap/internal/spec"
)

// version is set with -ldflags "-X main.version=…"; falls back to build info.
var version = "dev"

const usage = `archmap - interactive function-level architecture map

Usage:
  archmap extract [--root DIR] [--base REF] [--tests] [--no-src] --out facts.json
  archmap render  --facts facts.json --out arch.html [--no-src]
  archmap map     [--root DIR] [--base REF] [--tests] [--out FILE] [--open] [--no-src]
  archmap serve   [--root DIR] [--base REF] [--tests] [--addr 127.0.0.1:0] [--open] [--no-src]
  archmap lint    [--root DIR] [--format text|json] [--tests] [--strict] [--missing-docs]
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
		err = cmdServe(args[1:], stdout, stderr)
	case "lint":
		return cmdLint(args[1:], stdout, stderr)
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

func extractFacts(root, base string, tests, src bool) (*facts.Facts, error) {
	return extract.Run(extract.Options{Root: root, Base: base, WithTests: tests, Parsers: all.Parsers(), Src: src})
}

// srcFlags registers --src (default on) and --no-src; the result says whether
// function bodies go into the output.
func srcFlags(fs *flag.FlagSet) func() bool {
	src := fs.Bool("src", true, "put function bodies into the output")
	no := fs.Bool("no-src", false, "leave function bodies out (for a page that leaves your machine)")
	return func() bool { return *src && !*no }
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
	withSrc := srcFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("--out is required")
	}
	f, err := extractFacts(*root, *base, *tests, withSrc())
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
	out := fs.String("out", "", "output HTML (required)")
	withSrc := srcFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *factsPath == "" || *out == "" {
		return errors.New("--facts and --out are required")
	}
	if err := render.Files(*factsPath, *out, !withSrc()); err != nil {
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
	out := fs.String("out", "", "output HTML (default: <root>/.archmap/arch.html)")
	open := fs.Bool("open", false, "open the result in the default browser")
	withSrc := srcFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	f, err := extractFacts(*root, *base, *tests, withSrc())
	if err != nil {
		return err
	}
	if *out == "" {
		*out = filepath.Join(*root, ".archmap", "arch.html")
	}
	// arch/*.arch is shown read-only: the static map has no editor
	set, err := spec.Load(*root)
	if err != nil {
		return err
	}
	fb, err := marshal(spec.MergeStates(f, set), "")
	if err != nil {
		return err
	}
	mb, err := autosem.JSONWith(f, set)
	if err != nil {
		return err
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
	fmt.Fprintln(stderr, "archmap: "+lint.Docs(f).Line()+" (archmap lint --missing-docs lists the gaps)")
	if *open {
		return openBrowser(*out)
	}
	return nil
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

func cmdServe(args []string, stdout, stderr io.Writer) error {
	fs := newFlags("serve", stderr)
	root := fs.String("root", ".", "repository root")
	base := fs.String("base", "", "git ref to diff against (default: merge-base with the default branch)")
	tests := fs.Bool("tests", false, "include test files")
	addr := fs.String("addr", "127.0.0.1:0", "listen address (loopback only)")
	open := fs.Bool("open", false, "open the map in the default browser")
	withSrc := srcFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ln, err := serve.Listen(*addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	srv, err := serve.New(serve.Options{Root: *root, Base: *base, WithTests: *tests, Parsers: all.Parsers(), NoSrc: !withSrc()})
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String() + "/"
	fmt.Fprintln(stdout, "archmap serve:", url)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go srv.Watch(ctx)
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() { <-ctx.Done(); _ = hs.Close() }()
	if *open {
		if err := openBrowser(url); err != nil {
			fmt.Fprintln(stderr, "archmap: open:", err)
		}
	}
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
