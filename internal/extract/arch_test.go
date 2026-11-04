package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidarbn/archmap/internal/lang/all"
)

// The comment above a declaration reaches annot in every language.
func TestArchNotesAllLanguages(t *testing.T) {
	cases := map[string]string{
		"a.go":    "package a\n\n// " + "arch:flow f \"Flow\"\n// arch:ext Kaspi\nfunc Run() {}\n",
		"a.py":    "# arch:flow f \"Flow\"\n# arch:ext Kaspi\n@deco\ndef run():\n    pass\n",
		"a.js":    "/**\n * arch:flow f \"Flow\"\n * arch:ext Kaspi\n */\nfunction run() {}\n",
		"a.ts":    "// arch:flow f \"Flow\"\n// arch:ext Kaspi\nexport function run(): void {}\n",
		"a.kt":    "// arch:flow f \"Flow\"\n// arch:ext Kaspi\nfun run() {}\n",
		"a.swift": "// arch:flow f \"Flow\"\n// arch:ext Kaspi\nfunc run() {}\n",
		"a.java":  "class A {\n    // arch:flow f \"Flow\"\n    // arch:ext Kaspi\n    void run() {}\n}\n",
		"a.rs":    "/// arch:flow f \"Flow\"\n/// arch:ext Kaspi\nfn run() {}\n",
		"a.rb":    "class A\n  # arch:flow f \"Flow\"\n  # arch:ext Kaspi\n  def run\n  end\nend\n",
		"a.c":     "// arch:flow f \"Flow\"\n// arch:ext Kaspi\nvoid run(void) {}\n",
		"a.cpp":   "// arch:flow f \"Flow\"\n// arch:ext Kaspi\nvoid run() {}\n",
		"a.lua":   "-- arch:flow f \"Flow\"\n-- arch:ext Kaspi\nfunction run() end\n",
		"a.sh":    "# arch:flow f \"Flow\"\n# arch:ext Kaspi\nrun() {\n  echo\n}\n",
		"a.zig":   "// arch:flow f \"Flow\"\n// arch:ext Kaspi\nfn run() void {}\n",
		"a.ml":    "(* arch:flow f \"Flow\"\n   arch:ext Kaspi *)\nlet run () = ()\n",
		"a.cs":    "class A {\n    // arch:flow f \"Flow\"\n    // arch:ext Kaspi\n    void Run() {}\n}\n",
		"a.php":   "<?php\n// arch:flow f \"Flow\"\n// arch:ext Kaspi\nfunction run() {}\n",
		"a.scala": "object A {\n  // arch:flow f \"Flow\"\n  // arch:ext Kaspi\n  def run(): Unit = {}\n}\n",
		"a.dart":  "// arch:flow f \"Flow\"\n// arch:ext Kaspi\nvoid run() {}\n",
		"a.ex":    "defmodule A do\n  # arch:flow f \"Flow\"\n  # arch:ext Kaspi\n  def run do\n  end\nend\n",
		"a.hs":    "-- arch:flow f \"Flow\"\n-- arch:ext Kaspi\nrun :: IO ()\nrun = return ()\n",
	}
	for name, body := range cases {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		f, err := Run(Options{Root: root, Parsers: all.Parsers()})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for fid, fn := range f.Functions {
			if fn.Name == "run" || fn.Name == "Run" {
				found = true
				if fn.Arch == nil || len(fn.Arch.Flows) != 1 || fn.Arch.Flows[0].ID != "f" || len(fn.Arch.Ext) != 1 {
					t.Errorf("%s (%s): arch = %+v", name, fid, fn.Arch)
				}
			}
		}
		if !found {
			t.Errorf("%s: function not found: %v", name, f.Functions)
		}
	}
}
