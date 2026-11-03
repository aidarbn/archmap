package ts

import (
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestAllGrammarsParse(t *testing.T) {
	snippets := map[Lang]string{
		Python:     "def f():\n    return g()\n",
		Go:         "package p\nfunc F() { G() }\n",
		JavaScript: "function f() { return g(); }\n",
		TypeScript: "function f(a: number): number { return g(a); }\n",
		TSX:        "const A = () => <div>{g()}</div>;\n",
		Swift:      "func f() { g() }\n",
		Kotlin:     "fun f() { g() }\n",
	}
	for _, l := range All {
		tr, err := Parse(l, []byte(snippets[l]))
		if err != nil {
			t.Fatalf("%s: %v", l, err)
		}
		if tr.Root().HasError() {
			t.Errorf("%s: unexpected syntax error: %s", l, tr.Root().ToSexp())
		}
		calls := 0
		Walk(tr.Root(), func(n *sitterNode) bool {
			if k := n.Kind(); k == "call" || k == "call_expression" {
				calls++
			}
			return true
		})
		if calls == 0 {
			t.Errorf("%s: no call node found in %s", l, tr.Root().ToSexp())
		}
		tr.Close()
	}
}

type sitterNode = sitter.Node
