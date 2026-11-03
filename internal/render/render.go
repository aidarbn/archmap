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
)

//go:embed viewer.html
var viewer string

// Viewer returns the embedded viewer.html template.
func Viewer() string { return viewer }

// HTML renders the page from facts.json and (optionally empty/nil) model JSON.
func HTML(factsJSON, modelJSON []byte) ([]byte, error) {
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
	}{factsJSON, modelJSON}); err != nil {
		return nil, err
	}
	data := strings.ReplaceAll(strings.TrimRight(payload.String(), "\n"), "<", "\\"+"u003c")
	title := model.Title
	if title == "" {
		title = meta.Repo + " — архитектура"
	}
	html := strings.ReplaceAll(viewer, "/*ARCH_DATA*/null", data)
	html = strings.ReplaceAll(html, "<!--TITLE-->", strings.ReplaceAll(title, "<", "&lt;"))
	return []byte(html), nil
}

// Files renders factsPath (+ modelPath if it exists) into out, creating parent directories.
func Files(factsPath, modelPath, out string) error {
	fb, err := os.ReadFile(factsPath)
	if err != nil {
		return err
	}
	var mb []byte
	if modelPath != "" {
		if b, err := os.ReadFile(modelPath); err == nil {
			mb = b
		}
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
