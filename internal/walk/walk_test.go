package walk_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidarbn/archmap/internal/lang/all"
	"github.com/aidarbn/archmap/internal/walk"
)

func TestListUsesRegistryExtensions(t *testing.T) {
	root := t.TempDir()
	files := map[string]bool{
		"a/Main.java": true, "src/lib.rs": true, "app/x.rb": true, "p/m.py": true,
		"README.md": false, "target/x.rs": false, "src/test/FooTest.java": false,
		"spec/x_spec.rb": false, "tests/it.rs": false,
	}
	for p := range files {
		full := filepath.Join(root, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := map[string]bool{}
	for _, p := range walk.List(root, walk.ExtSet(all.Parsers()), false) {
		got[p] = true
	}
	for p, want := range files {
		if got[p] != want {
			t.Errorf("%s: listed=%v want %v", p, got[p], want)
		}
	}
}
