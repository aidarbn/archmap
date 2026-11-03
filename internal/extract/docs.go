package extract

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/gitstat"
	"github.com/aidarbn/archmap/internal/lang"
)

// moduleDoc picks the description of one module: the file-level doc of its most
// representative file (see docRank), else the README of the directory.
func moduleDoc(root, module string, files []string, fileDoc map[string]string) string {
	best, bestRank := "", 1<<30
	for _, f := range files { // files are sorted: ties go to the alphabetically first
		d := fileDoc[f]
		if d == "" {
			continue
		}
		if r := docRank(f, module, len(files)); r < noRank && r < bestRank {
			best, bestRank = d, r
		}
	}
	if best == "" { // package-info.java is a source file that may not be in the list
		if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(module), "package-info.java")); err == nil {
			best = lang.FileDoc("package-info.java", b)
		}
	}
	if best != "" {
		return best
	}
	return readmeDoc(filepath.Join(root, filepath.FromSlash(module)))
}

// noRank and above marks a file that cannot describe its module.
const noRank = 1 << 20

// docRank orders the files of a module by how well they describe it (lower is
// better); a negative-free large value means the file is not a candidate.
func docRank(file, module string, nfiles int) int {
	name := path.Base(file)
	stem := strings.TrimSuffix(name, path.Ext(name))
	dir := path.Base(module)
	if module == "." {
		dir = ""
	}
	switch path.Ext(file) {
	case ".go":
		if name == "doc.go" {
			return 0
		}
		return 1
	case ".py", ".pyi":
		switch {
		case stem == "__init__":
			return 0
		case stem == "main" || stem == "app" || stem == dir:
			return 1
		}
		return 2
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue":
		switch {
		case stem == "index":
			return 0
		case stem == dir:
			return 1
		case nfiles == 1:
			return 2
		}
		return 1 << 20
	case ".rs":
		switch name {
		case "mod.rs", "lib.rs", "main.rs":
			return 0
		}
		return 1 << 20
	case ".java":
		if name == "package-info.java" {
			return 0
		}
		return 1 << 20
	}
	if stem == dir {
		return 0
	}
	return 3
}

var (
	badgeRe  = regexp.MustCompile(`!\[|\[!\[|<img|shields\.io|badge`)
	linkRe   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	htmlRe   = regexp.MustCompile(`<[^>]+>`)
	ruleRe   = regexp.MustCompile(`^(=+|-+|\*{3,}|_{3,})$`)
	readmeFn = []string{"README.md", "readme.md", "Readme.md", "README", "README.rst", "README.txt", "README.markdown"}
)

// readmeDoc returns the first prose paragraph of the README in dir.
func readmeDoc(dir string) string {
	for _, n := range readmeFn {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		if d := readmeParagraph(string(b)); d != "" {
			return d
		}
	}
	return ""
}

// readmeParagraph returns the first paragraph that is prose: not a heading,
// badge, image, HTML, table, list or code.
func readmeParagraph(text string) string {
	var para []string
	inFence := false
	flush := func() string {
		if len(para) == 0 {
			return ""
		}
		p := strings.Join(para, " ")
		para = nil
		p = linkRe.ReplaceAllString(p, "$1")
		p = htmlRe.ReplaceAllString(p, "")
		p = strings.NewReplacer("**", "", "__", "", "`", "").Replace(p)
		return lang.Describe(p)
	}
	for _, l := range strings.Split(text, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			para = nil
			continue
		}
		if inFence {
			continue
		}
		if ruleRe.MatchString(t) && len(para) == 1 { // setext heading underline
			para = nil
			continue
		}
		skip := t == "" || strings.HasPrefix(t, "See ") || strings.HasPrefix(t, "Note:") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "<") || strings.HasPrefix(t, "|") ||
			strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "+ ") || strings.HasPrefix(t, ">") ||
			strings.HasPrefix(t, "[") && strings.HasSuffix(t, ":") || strings.HasPrefix(t, "..") ||
			ruleRe.MatchString(t) || badgeRe.MatchString(t) || strings.HasPrefix(t, "http")
		if skip {
			if d := flush(); d != "" {
				return d
			}
			para = nil
			continue
		}
		para = append(para, t)
	}
	return flush()
}

// projectInfo determines the project name and description of the repository.
func projectInfo(root string) *facts.Project {
	p := &facts.Project{}
	if out, ok := gitstat.Git(root, "remote", "get-url", "origin"); ok {
		p.Name = lastSegment(strings.TrimSpace(out))
	}
	var desc string
	setMeta := func(name, d string) {
		if p.Name == "" {
			p.Name = name
		}
		if desc == "" {
			desc = d
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[0] == "module" {
				setMeta(lastSegment(strings.Trim(f[1], `"`)), "")
				break
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var pj struct{ Name, Description string }
		if json.Unmarshal(b, &pj) == nil {
			setMeta(lastSegment(pj.Name), lang.Shorten(pj.Description))
		}
	}
	for _, c := range []struct{ file, section string }{
		{"Cargo.toml", "package"}, {"pyproject.toml", "project"}, {"pyproject.toml", "tool.poetry"},
	} {
		if b, err := os.ReadFile(filepath.Join(root, c.file)); err == nil {
			n, d := tomlSection(string(b), c.section)
			setMeta(n, lang.Shorten(d))
		}
	}
	if p.Name == "" {
		p.Name = filepath.Base(root)
	}
	p.Description = readmeDoc(root)
	if p.Description == "" {
		p.Description = desc
	}
	return p
}

// lastSegment returns the last path (or scp-style host:path) segment of s
// without a ".git" suffix.
func lastSegment(s string) string {
	s = strings.TrimSuffix(strings.TrimRight(s, "/"), ".git")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

var tomlKV = regexp.MustCompile(`^(name|description)\s*=\s*"([^"]*)"`)

// tomlSection reads name and description from one [section] of a TOML file.
func tomlSection(text, section string) (name, desc string) {
	in := false
	for _, l := range strings.Split(text, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") {
			in = t == "["+section+"]"
			continue
		}
		if !in {
			continue
		}
		if m := tomlKV.FindStringSubmatch(t); m != nil {
			if m[1] == "name" {
				name = m[2]
			} else {
				desc = m[2]
			}
		}
	}
	return
}
