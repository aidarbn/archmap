// Package render fills viewer.html with facts and the semantic model.
//
// Contract with viewer.html (unchanged from the Python original):
// the literal `/*ARCH_DATA*/null` is replaced by the compact JSON
// {"facts":…,"model":…} in which every "<" is written as the six characters backslash-u-0-0-3-c (so neither
// "</script>" nor "<!--<script" in a foreign repo's data can break out of the
// script block), and every `<!--TITLE-->` is replaced by the HTML-escaped title.
// Output is UTF-8.
package render

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidarbn/archmap/internal/autosem"
	"github.com/aidarbn/archmap/internal/facts"
)

//go:embed viewer.html
var viewer string

// Viewer returns the embedded viewer.html template.
func Viewer() string { return viewer }

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
	html := strings.ReplaceAll(viewer, "/*ARCH_DATA*/null", data)
	html = strings.ReplaceAll(html, "<!--TITLE-->", strings.ReplaceAll(title, "<", "&lt;"))
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
