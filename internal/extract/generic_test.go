package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/lang"
	"github.com/aidarbn/archmap/internal/lang/generic"
)

func genericRepo(t *testing.T, files map[string]string) *facts.Facts {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var ps []lang.Parser
	for _, g := range generic.Parsers() {
		ps = append(ps, g)
	}
	f, err := Run(Options{Root: root, Parsers: ps})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func imports(f *facts.Facts, m string) string { return strings.Join(f.Modules[m].Imports, ",") }

func TestRustImports(t *testing.T) {
	f := genericRepo(t, map[string]string{
		"Cargo.toml":              "[package]\nname = \"app\"\n",
		"src/main.rs":             "mod util;\nmod api;\nuse crate::db::Pool;\nfn main() { util::go(); }\n",
		"src/util.rs":             "pub fn go() {}\n",
		"src/db.rs":               "pub struct Pool;\n",
		"src/api/mod.rs":          "pub mod handlers;\n",
		"src/api/handlers.rs":     "use super::Thing;\nuse crate::util::go;\nuse crate::db::*;\npub fn h() {}\n",
		"crates/lib-x/Cargo.toml": "[package]\nname = \"lib-x\"\n",
		"crates/lib-x/src/lib.rs": "pub fn x() {}\n",
		"crates/app/Cargo.toml":   "[package]\nname = \"app2\"\n",
		"crates/app/src/main.rs":  "use lib_x::x;\nfn main() { x(); }\n",
	})
	if got := imports(f, "src"); got != "src/api" {
		t.Errorf("src imports = %q", got)
	}
	if got := imports(f, "src/api"); got != "src" {
		t.Errorf("src/api imports = %q", got)
	}
	if got := imports(f, "crates/app/src"); got != "crates/lib-x/src" {
		t.Errorf("workspace crate import = %q", got)
	}
}

func TestRubyImports(t *testing.T) {
	f := genericRepo(t, map[string]string{
		"lib/app.rb":       "require 'app/base'\nrequire_relative 'app/ext/x'\nrequire 'json'\n",
		"lib/app/base.rb":  "class Base; end\n",
		"lib/app/ext/x.rb": "class X; end\n",
		"bin/run.rb":       "require 'app'\n",
	})
	if got := imports(f, "lib"); got != "lib/app,lib/app/ext" {
		t.Errorf("lib imports = %q", got)
	}
	if got := imports(f, "bin"); got != "lib" {
		t.Errorf("bin imports = %q", got)
	}
}

func TestJavaImportsAndEntries(t *testing.T) {
	f := genericRepo(t, map[string]string{
		"src/main/java/com/acme/App.java": "package com.acme;\nimport com.acme.web.Ctl;\nimport static com.acme.util.Strs.up;\nclass App { public static void main(String[] a) { } }\n",
		"src/main/java/com/acme/web/Ctl.java": `package com.acme.web;
import com.acme.util.*;
class Ctl {
  @GetMapping("/owners/{id}")
  public String show() { return ""; }
  @RequestMapping(value = "/x", method = RequestMethod.POST)
  public String x() { return ""; }
  @Scheduled(fixedRate = 5)
  void tick() { }
  void helper() { }
}
`,
		"src/main/java/com/acme/util/Strs.java": "package com.acme.util;\nclass Strs { static String up(String s) { return s; } }\n",
	})
	if got := imports(f, "src/main/java/com/acme"); got != "src/main/java/com/acme/util,src/main/java/com/acme/web" {
		t.Errorf("imports = %q", got)
	}
	if got := imports(f, "src/main/java/com/acme/web"); got != "src/main/java/com/acme/util" {
		t.Errorf("wildcard import = %q", got)
	}
	want := map[string]string{
		"App.main": "main:App.main", "Ctl.show": "http:GET /owners/{id}", "Ctl.x": "http:POST /x", "Ctl.tick": "task:Scheduled",
	}
	got := map[string]string{}
	for id, fn := range f.Functions {
		if fn.Entry != nil {
			got[id[strings.Index(id, "::")+2:]] = fn.Entry.Kind + ":" + fn.Entry.Labels[0]
		}
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("entry %s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("extra entries: %v", got)
	}
}

func TestRustAndRailsEntries(t *testing.T) {
	f := genericRepo(t, map[string]string{
		"src/main.rs": `#[actix_web::get("/ping")]
async fn ping() {}
#[post("/items")]
async fn add() {}
async fn list() {}
fn router() { Router::new().route("/items", get(list).post(add)); }
#[tokio::main]
async fn main() {}
fn plain() {}
`,
		"app/controllers/owners_controller.rb": "class OwnersController < ApplicationController\n  def index\n  end\n  def show\n  end\n  private\n  def owner_params\n  end\n  def load\n  end\nend\n",
	})
	got := map[string]string{}
	for id, fn := range f.Functions {
		if fn.Entry != nil {
			got[id[strings.Index(id, "::")+2:]] = fn.Entry.Kind + ":" + strings.Join(fn.Entry.Labels, "|")
		}
	}
	want := map[string]string{
		"ping": "http:GET /ping", "add": "http:POST /items", "list": "http:GET /items", "main": "main:main",
		"OwnersController.index": "http:OwnersController#index", "OwnersController.show": "http:OwnersController#show",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("entry %s = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("entries: %v", got)
	}
}

func TestPathLikeGenericRule(t *testing.T) {
	files := []string{
		"src/main.c", "include/util.h", "src/net/conn.h", "web/index.php", "web/lib/db.php", "init.lua", "scripts/x.lua",
		"lib/my_app/foo_bar.ex", "app/lib/Data/Map.hs", "lib/src/widget.dart", "app/Models/User.cs", "tools/run.sh",
	}
	var mods []string
	seen := map[string]bool{}
	for _, p := range files {
		if m := facts.ModuleOf(p); !seen[m] {
			seen[m] = true
			mods = append(mods, m)
		}
	}
	r := newResolver(files, mods, nil, nil)
	cases := []struct{ lang, imp, file, want string }{
		{"c", "util.h", "src/main.c", "include"},
		{"c", "net/conn.h", "src/main.c", "src/net"},
		{"c", "stdio.h", "src/main.c", ""},
		{"php", "lib/db.php", "web/index.php", "web/lib"},
		{"php", "./lib/db.php", "web/index.php", "web/lib"},
		{`php`, `App\Models\User`, "web/index.php", "app/Models"},
		{"lua", "scripts.x", "init.lua", "scripts"},
		{"elixir", "MyApp.FooBar", "lib/other.ex", "lib/my_app"},
		{"haskell", "Data.Map", "app/Main.hs", "app/lib/Data"},
		{"dart", "package:myapp/src/widget.dart", "lib/main.dart", "lib/src"},
		{"dart", "dart:async", "lib/main.dart", ""},
		{"csharp", "App.Models", "x/Program.cs", "app/Models"},
		{"bash", "./run.sh", "tools/other.sh", "tools"},
	}
	for _, c := range cases {
		if got := r.resolve(c.imp, c.file, c.lang); got != c.want {
			t.Errorf("%s %q from %s = %q, want %q", c.lang, c.imp, c.file, got, c.want)
		}
	}
}

func TestUnparsedFilesStayOnTheMap(t *testing.T) {
	f := genericRepo(t, map[string]string{
		"a/x.ex":      "defmodule X do\nend\n",
		"a/y.exs":     "1\n",
		"b/z.m":       "int main() {}\n",
		"b/w.rs":      "fn w() {}\n",
		"c/readme.md": "x\n",
	})
	if f.Unparsed[".exs"] != 1 || f.Unparsed[".m"] != 1 {
		t.Errorf("unparsed = %v", f.Unparsed)
	}
	if got := f.Modules["a"].Files; len(got) == 0 {
		t.Errorf("module a has no files: %v", f.Modules)
	}
	if _, ok := f.Modules["c"]; ok {
		t.Error("markdown must not be a module")
	}
	if !strings.Contains(Summary(f), "без разбора:") {
		t.Errorf("summary lacks the unparsed line:\n%s", Summary(f))
	}
}
