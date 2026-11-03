package generic

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"

	"github.com/aidarbn/archmap/internal/lang"
)

func parse(t *testing.T, name, src string) lang.File {
	t.Helper()
	for _, s := range Specs() {
		if s.Name == name {
			f, err := New(s).Parse("x", []byte(src))
			if err != nil {
				t.Fatal(err)
			}
			return f
		}
	}
	t.Fatalf("no spec %s", name)
	return lang.File{}
}

func dump(f lang.File) string {
	var b strings.Builder
	for _, fn := range f.Funcs {
		fmt.Fprintf(&b, "%s|%s|%d-%d|%s|%s|", fn.Cls, fn.Name, fn.Start, fn.End, fn.Sig, fn.Doc)
		for _, c := range fn.Calls {
			fmt.Fprintf(&b, "%s>%s,", c.Recv, c.Name)
		}
		b.WriteString("\n")
	}
	for _, i := range f.Imports {
		fmt.Fprintf(&b, "import %s\n", i.Path)
	}
	return b.String()
}

func check(t *testing.T, got lang.File, want string) {
	t.Helper()
	if g := dump(got); g != want {
		t.Errorf("got:\n%s\nwant:\n%s", g, want)
	}
}

func TestJava(t *testing.T) {
	src := `package p;
import java.util.List;
import static org.x.Y.z;
import a.b.*;

/** Owner service. */
public class Owner {
    private final Repo repo;

    /**
     * Finds one.
     */
    @Override
    public Pet find(int id) {
        Pet p = repo.get(id);
        this.log(p);
        helper(id);
        return new Pet(p.name().trim());
    }

    Owner() { setup(); }

    interface Inner { void run(); }
}
`
	check(t, parse(t, "java", src), `Owner|find|13-19|@Override public Pet find(int id)|Finds one.|repo>get,this>log,>helper,>Pet,p>name,?>trim,
Owner|Owner|21-21|Owner()||>setup,
Inner|run|23-23|void run();||
import java.util.List
import org.x.Y.z
import a.b.*
`)
}

func TestRust(t *testing.T) {
	src := `use std::collections::{HashMap, hash_map::Entry as E};
use crate::util::*;
mod inner;
mod other { fn x() {} }

/// Makes a thing.
struct Thing;

impl Thing {
    /// Build it.
    #[inline]
    pub fn new() -> Self {
        let s = Self::make();
        util::run(s.field.go());
        s
    }
}

trait Run { fn run(&self) { self.step(); } }

fn main() {
    let t = Thing::new();
    t.go::<u8>();
}
`
	check(t, parse(t, "rust", src), `other|x|4-4|fn x()||
Thing|new|12-16|pub fn new() -> Self|Build it.|self>make,util>run,s.field>go,
Run|run|19-19|fn run(&self)||self>step,
|main|21-24|fn main()||Thing>new,t>go,
import std::collections::HashMap
import std::collections::hash_map::Entry
import crate::util::*
import self::inner
`)
}

func TestRuby(t *testing.T) {
	src := `require 'json'
require_relative 'lib/helper'
require_relative './b'

module Shop
  # Order class.
  class Order
    # Total price.
    def total(tax)
      items.sum { |i| i.price }
      Calc.round(tax)
      @repo&.save(self)
      puts "x"
    end

    def self.build
      new.total(1)
    end
  end
end
`
	check(t, parse(t, "ruby", src), `Order|total|9-14|def total(tax)|Total price.|items>sum,i>price,Calc>round,@repo>save,>puts,
Order|build|16-18|def self.build||>total,
import json
import ./lib/helper
import ./b
`)
}

// A new language is one Spec: grammar, query, extensions. The Go grammar stands
// in for "any other" grammar here.
func TestNewLanguageIsOneSpec(t *testing.T) {
	p := New(Spec{
		Name: "gox", Extensions: []string{".gox"},
		Grammar: func() unsafe.Pointer { return golang.Language() },
		Tags: `(function_declaration name: (identifier) @name) @definition.function
(call_expression function: (identifier) @name) @reference.call`,
	})
	f, err := p.Parse("a.gox", []byte("package a\nfunc A() {\n\tB()\n}\nfunc B() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	check(t, f, "|A|2-4|func A()||>B,\n|B|5-5|func B()||\n")
	if p.Name() != "gox" || p.Extensions()[0] != ".gox" {
		t.Error("meta")
	}
}

func TestBadQuery(t *testing.T) {
	p := New(Spec{Name: "x", Grammar: func() unsafe.Pointer { return golang.Language() }, Tags: "(nope"})
	if _, err := p.Parse("a", []byte("package a")); err == nil {
		t.Error("want error")
	}
}

func TestBrokenInput(t *testing.T) {
	for _, s := range Specs() {
		if _, err := New(s).Parse("x", []byte("class {{ fn ( def end ;;; \x00")); err != nil {
			t.Errorf("%s: %v", s.Name, err)
		}
	}
}
