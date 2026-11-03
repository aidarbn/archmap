package extract

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aidarbn/archmap/internal/lang"
)

func put(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestModuleDocSources(t *testing.T) {
	root := t.TempDir()
	put(t, root, "py/__init__.py", "\"\"\"Package docstring wins here.\"\"\"\n")
	put(t, root, "py/a.py", "\"\"\"Other module docstring.\"\"\"\n")
	put(t, root, "pyonly/b.py", "\"\"\"Only the first module speaks.\"\"\"\n")
	put(t, root, "pyonly/a.py", "")
	put(t, root, "js/index.ts", "/** Index of the widgets area. */\n")
	put(t, root, "js/other.ts", "/** Other file that should lose. */\n")
	put(t, root, "jsmany/a.ts", "/** Not an index so ignored. */\n")
	put(t, root, "jsmany/b.ts", "")
	put(t, root, "jsone/solo.ts", "/** The only file describes the module. */\n")
	put(t, root, "rs/lib.rs", "//! Rust crate docs live in lib.rs.\n")
	put(t, root, "jv/package-info.java", "/** Java package docs from package-info. */\npackage jv;\n")
	put(t, root, "kt/kt.kt", "/** Kotlin file named like the directory. */\n")
	put(t, root, "kt/z.kt", "/** Kotlin other file loses. */\n")
	put(t, root, "readme/README.md", "# Title\n\n[![Build](https://x/badge.svg)](https://x)\n\nThe readme paragraph describes this folder. Second sentence.\n\n## More\n")
	put(t, root, "lic/x.ts", "// Copyright 2020 Acme\nexport {}\n")
	put(t, root, "lic/README.md", "Falls back to readme when the header is a licence.\n")

	files := map[string][]string{
		"py": {"py/__init__.py", "py/a.py"}, "pyonly": {"pyonly/a.py", "pyonly/b.py"},
		"js": {"js/index.ts", "js/other.ts"}, "jsmany": {"jsmany/a.ts", "jsmany/b.ts"}, "jsone": {"jsone/solo.ts"},
		"rs": {"rs/lib.rs"}, "jv": {}, "kt": {"kt/kt.kt", "kt/z.kt"}, "readme": {}, "lic": {"lic/x.ts"},
	}
	want := map[string]string{
		"py":     "Package docstring wins here.",
		"pyonly": "Only the first module speaks.",
		"js":     "Index of the widgets area.",
		"jsmany": "",
		"jsone":  "The only file describes the module.",
		"rs":     "Rust crate docs live in lib.rs.",
		"jv":     "Java package docs from package-info.",
		"kt":     "Kotlin file named like the directory.",
		"readme": "The readme paragraph describes this folder. Second sentence.",
		"lic":    "Falls back to readme when the header is a licence.",
	}
	for m, fs := range files {
		docs := map[string]string{}
		for _, f := range fs {
			b, _ := os.ReadFile(filepath.Join(root, f))
			docs[f] = lang.FileDoc(f, b)
		}
		if got := moduleDoc(root, m, fs, docs); got != want[m] {
			t.Errorf("%s: got %q, want %q", m, got, want[m])
		}
	}
}

func TestReadmeParagraphSkipsNoise(t *testing.T) {
	text := "ripgrep (rg)\n------------\n[![ci](https://x/badge.svg)](https://x)\n\n<p align=\"center\"><img src=\"x.png\"></p>\n\nSee the docs here:\n\nripgrep is a [line-oriented](https://x) search tool. It is **fast**.\n"
	if got, want := readmeParagraph(text), "ripgrep is a line-oriented search tool. It is fast."; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProjectName(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"go.mod", map[string]string{"go.mod": "module github.com/acme/tool/v2\n\ngo 1.22\n"}, "v2"},
		{"package.json", map[string]string{"package.json": `{"name":"@acme/web-app","description":"Web app for orders. Fast."}`}, "web-app"},
		{"cargo", map[string]string{"Cargo.toml": "[package]\nname = \"crabby\"\ndescription = \"A crab tool.\"\n"}, "crabby"},
		{"pyproject", map[string]string{"pyproject.toml": "[project]\nname = \"pyproj\"\ndescription = \"A Python thing for people.\"\n"}, "pyproj"},
		{"dirname", map[string]string{}, "<dir>"},
	}
	for _, c := range cases {
		root := filepath.Join(t.TempDir(), "plain-dir")
		for n, s := range c.files {
			put(t, root, n, s)
		}
		os.MkdirAll(root, 0o755)
		p := projectInfo(root)
		want := c.want
		if want == "<dir>" {
			want = "plain-dir"
		}
		if p.Name != want {
			t.Errorf("%s: name %q, want %q", c.name, p.Name, want)
		}
	}
	root := t.TempDir()
	put(t, root, "package.json", `{"name":"x","description":"Web app for orders. Fast."}`)
	if p := projectInfo(root); p.Description != "Web app for orders. Fast." {
		t.Errorf("package.json description: %q", p.Description)
	}
	put(t, root, "README.md", "# X\n\nReadme beats the manifest description. Yes.\n")
	if p := projectInfo(root); p.Description != "Readme beats the manifest description. Yes." {
		t.Errorf("readme description: %q", p.Description)
	}
}

func TestProjectNameFromOriginBeatsGoMod(t *testing.T) {
	root := t.TempDir()
	put(t, root, "go.mod", "module example.com/other\n")
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "remote", "add", "origin", "git@github.com:acme/cool-repo.git")
	if p := projectInfo(root); p.Name != "cool-repo" {
		t.Errorf("ssh remote: %q", p.Name)
	}
	gitIn(t, root, "remote", "set-url", "origin", "https://github.com/acme/other-name.git")
	if p := projectInfo(root); p.Name != "other-name" {
		t.Errorf("https remote: %q", p.Name)
	}
}

func TestRunFillsProjectAndModuleDoc(t *testing.T) {
	root := filepath.Join(t.TempDir(), "dirname")
	put(t, root, "svc/__init__.py", "\"\"\"Service layer for the demo project.\"\"\"\n")
	put(t, root, "svc/a.py", "def f -> \n")
	f, err := Run(Options{Root: root, Parsers: []lang.Parser{fakeParser{}}})
	if err != nil {
		t.Fatal(err)
	}
	if f.Repo != "dirname" || f.Project == nil || f.Project.Name != "dirname" {
		t.Errorf("repo/project: %q %+v", f.Repo, f.Project)
	}
	if got := f.Modules["svc"].Doc; got != "Service layer for the demo project." {
		t.Errorf("module doc: %q", got)
	}
}
