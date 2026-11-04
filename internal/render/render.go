// Package render builds the single-file viewer and fills it with facts and the
// semantic model.
//
// The viewer is kept as several sources in viewer/ (see docs/viewer.md) and
// assembled once into one HTML template: shell.html gets the CSS in place of
// `<!--@CSS-->` and the JavaScript in place of `<!--@JS-->`. The page stays one
// offline file with no external resources.
//
// Contract with the template (unchanged from the Python original):
// the literal `/*ARCH_DATA*/null` is replaced by the compact JSON
// {"facts":…,"model":…} in which every "<" is written as the six characters backslash-u-0-0-3-c (so neither
// "</script>" nor "<!--<script" in a foreign repo's data can break out of the
// script block), and every `<!--TITLE-->` is replaced by the HTML-escaped title.
// Output is UTF-8.
package render

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/autosem"
	"github.com/aidarbn/archmap/internal/facts"
)

//go:embed viewer
var viewerFS embed.FS

// Assembly order. Core scripts share one scope and go first in this order;
// then every views/*.js in name order, each wrapped in its own block so a view
// cannot clash with another view; main.js (start-up) goes last.
var (
	coreCSS = []string{"viewer.css"}
	coreJS  = []string{"core.js", "scene.js", "layout.js", "map.js", "playback.js", "panel.js", "nav.js", "zoom.js"}
	lastJS  = "main.js"
)

const (
	cssSlot  = "<!--@CSS-->"
	jsSlot   = "<!--@JS-->"
	dataSlot = "/*ARCH_DATA*/null"
	titleTag = "<!--TITLE-->"
)

var viewer = mustAssemble(viewerFS)

// Viewer returns the assembled viewer template (placeholders not filled).
func Viewer() string { return viewer }

func mustAssemble(fsys fs.FS) string {
	s, err := assemble(fsys)
	if err != nil {
		panic("render: viewer assembly: " + err.Error())
	}
	return s
}

// assemble concatenates the viewer sources from fsys (rooted above viewer/).
func assemble(fsys fs.FS) (string, error) {
	read := func(name string) (string, error) {
		b, err := fs.ReadFile(fsys, path.Join("viewer", name))
		return string(b), err
	}
	shell, err := read("shell.html")
	if err != nil {
		return "", err
	}
	for _, slot := range []string{cssSlot, jsSlot, dataSlot} {
		if n := strings.Count(shell, slot); n != 1 {
			return "", fmt.Errorf("shell.html: %s occurs %d times, want 1", slot, n)
		}
	}
	if strings.Index(shell, dataSlot) > strings.Index(shell, jsSlot) {
		return "", fmt.Errorf("shell.html: %s must precede %s", dataSlot, jsSlot)
	}
	viewCSS, err := fs.Glob(fsys, "viewer/views/*.css")
	if err != nil {
		return "", err
	}
	viewJS, err := fs.Glob(fsys, "viewer/views/*.js")
	if err != nil {
		return "", err
	}
	sort.Strings(viewCSS)
	sort.Strings(viewJS)
	rel := func(list []string) []string {
		out := make([]string, len(list))
		for i, p := range list {
			out[i] = strings.TrimPrefix(p, "viewer/")
		}
		return out
	}

	var css strings.Builder
	for _, name := range append(append([]string{}, coreCSS...), rel(viewCSS)...) {
		src, err := read(name)
		if err != nil {
			return "", err
		}
		if err := checkPart(name, src, "</style"); err != nil {
			return "", err
		}
		fmt.Fprintf(&css, "/* --- %s --- */\n%s\n", name, strings.TrimRight(src, "\n"))
	}

	var js strings.Builder
	add := func(name string, block bool) error {
		src, err := read(name)
		if err != nil {
			return err
		}
		if err := checkPart(name, src, "</script"); err != nil {
			return err
		}
		src = strings.TrimRight(src, "\n")
		if block {
			fmt.Fprintf(&js, "/* --- %s --- */\n{\n%s\n}\n", name, src)
		} else {
			fmt.Fprintf(&js, "/* --- %s --- */\n%s\n", name, src)
		}
		return nil
	}
	for _, name := range coreJS {
		if err := add(name, false); err != nil {
			return "", err
		}
	}
	for _, name := range rel(viewJS) {
		if err := add(name, true); err != nil {
			return "", err
		}
	}
	if err := add(lastJS, false); err != nil {
		return "", err
	}

	out := strings.Replace(shell, cssSlot, "<style>\n"+css.String()+"</style>", 1)
	out = strings.Replace(out, jsSlot, js.String(), 1)
	return out, nil
}

// checkPart rejects a source that could break the page: a closing tag of its
// own block or a placeholder that render substitutes.
func checkPart(name, src, closer string) error {
	if strings.Contains(strings.ToLower(src), closer) {
		return fmt.Errorf("%s: contains %q", name, closer)
	}
	for _, p := range []string{dataSlot, titleTag, cssSlot, jsSlot} {
		if strings.Contains(src, p) {
			return fmt.Errorf("%s: contains placeholder %q", name, p)
		}
	}
	return nil
}

// HTML renders the page from facts.json and (optionally empty/nil) model JSON from autosem.
func HTML(factsJSON, modelJSON []byte) ([]byte, error) {
	return page(factsJSON, modelJSON, nil)
}

// Live describes the endpoints of `archmap serve`; when present in the data the
// viewer subscribes to Events and re-reads Data on every update.
type Live struct {
	Seq    int    `json:"seq"`
	Events string `json:"events"`
	Data   string `json:"data"`
	// Src is the endpoint that returns a function body by ?fid=; empty when bodies are off.
	Src string `json:"src,omitempty"`
}

// HTMLLive renders the page for `archmap serve`: same as HTML plus a "serve"
// key in the data. Static HTML (HTML, Files) is byte-for-byte unaffected.
func HTMLLive(factsJSON, modelJSON []byte, l Live) ([]byte, error) {
	return page(factsJSON, modelJSON, &l)
}

func page(factsJSON, modelJSON []byte, live *Live) ([]byte, error) {
	var meta struct {
		Repo string `json:"repo"`
	}
	if err := json.Unmarshal(factsJSON, &meta); err != nil {
		return nil, fmt.Errorf("facts: %w", err)
	}
	if len(bytes.TrimSpace(modelJSON)) == 0 {
		modelJSON = []byte("{}")
	}
	var model struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, fmt.Errorf("model: %w", err)
	}
	var payload bytes.Buffer
	enc := json.NewEncoder(&payload)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(struct {
		Facts json.RawMessage `json:"facts"`
		Model json.RawMessage `json:"model"`
		Serve *Live           `json:"serve,omitempty"`
	}{factsJSON, modelJSON, live}); err != nil {
		return nil, err
	}
	data := strings.ReplaceAll(strings.TrimRight(payload.String(), "\n"), "<", "\\"+"u003c")
	title := model.Title
	if title == "" {
		// language-neutral: the viewer appends the localized suffix after load
		title = meta.Repo
	}
	html := strings.Replace(viewer, dataSlot, data, 1)
	html = strings.ReplaceAll(html, titleTag, strings.ReplaceAll(title, "<", "&lt;"))
	return []byte(html), nil
}

// Files renders factsPath into out, creating parent directories; noSrc drops the
// function bodies from the data. The semantic
// layer is derived from the facts by autosem.
func Files(factsPath, out string, noSrc bool) error {
	fb, err := os.ReadFile(factsPath)
	if err != nil {
		return err
	}
	var f facts.Facts
	if err := json.Unmarshal(fb, &f); err != nil {
		return fmt.Errorf("facts: %w", err)
	}
	if noSrc {
		for id, fn := range f.Functions {
			fn.Src = ""
			f.Functions[id] = fn
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(&f); err != nil {
			return err
		}
		fb = buf.Bytes()
	}
	mb, err := autosem.JSON(&f)
	if err != nil {
		return err
	}
	html, err := HTML(fb, mb)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, html, 0o644)
}
