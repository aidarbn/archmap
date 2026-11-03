package generic

import "testing"

func TestC(t *testing.T) {
	src := `#include <stdio.h>
#include "util/helper.h"

struct point { int x; };
typedef struct { int a; } rec_t;

/* Adds numbers. */
static int add(int a, int b) {
    printf("x");
    return helper(a) + p->vt->go(b) + obj.run();
}

char *name(void) { return 0; }
`
	check(t, parse(t, "c", src), `|add|8-11|static int add(int a, int b)|Adds numbers.|>printf,>helper,p.vt>go,obj>run,
|name|13-13|char *name(void)||
import ./util/helper.h
`)
}

func TestCpp(t *testing.T) {
	src := `#include <vector>
#include "foo/bar.hpp"
namespace ns {
class Foo : public Base {
public:
  // Runs it.
  void run(int x) { helper(x); this->go(); obj.m(); std::move(x); }
  Foo() : a(1) {}
  ~Foo();
  int inl() const;
};
void Foo::out(int y) { bar(y); }
template <typename T> T tmpl(T t) { return t; }
struct S { void m() {} };
}
int main() { ns::tmpl(1); return 0; }
`
	check(t, parse(t, "cpp", src), `Foo|run|7-7|void run(int x)|Runs it.|>helper,this>go,obj>m,std>move,
Foo|Foo|8-8|Foo() : a(1)||
Foo|out|12-12|void Foo::out(int y)||>bar,
ns|tmpl|13-13|T tmpl(T t)||
S|m|14-14|void m()||
|main|16-16|int main()||ns>tmpl,
import ./foo/bar.hpp
`)
}

func TestLua(t *testing.T) {
	src := `local json = require("lib.json")
local u = require "util"
local M = {}

-- Adds.
function M.add(a, b)
  print(a)
  return helper(a) + M:other(b) + obj.field.run()
end

function M:method() self:go() end
local function priv(x) return x end
local f = function(y) return y end
M.g = function() end
local t = { cb = function() end }
function top() end
`
	check(t, parse(t, "lua", src), `M|add|6-9|function M.add(a, b)|Adds.|>helper,M>other,obj.field>run,
M|method|11-11|function M:method()||self>go,
|priv|12-12|local function priv(x)||
|f|13-13|f = function(y) return y end||
|g|14-14|M.g = function() end||
|cb|15-15|{ cb = function() end }||
|top|16-16|function top() end||
import lib.json
import util
`)
}

func TestBash(t *testing.T) {
	src := `#!/bin/bash
source ./lib/common.sh
. "$DIR/other.sh"

# Greets.
greet() {
  echo "hi"
  helper "$1"
  git status | grep x
}

function deploy {
  greet world
}
`
	check(t, parse(t, "bash", src), `|greet|6-10|greet()|Greets.|>helper,>git,>grep,
|deploy|12-14|function deploy||>greet,
import ./lib/common.sh
`)
}

func TestZig(t *testing.T) {
	src := `const std = @import("std");
const util = @import("util.zig");

/// Adds.
pub fn add(a: i32, b: i32) i32 {
    std.debug.print("x", .{});
    return helper(a) + util.run(b);
}

const Foo = struct {
    x: i32,
    pub fn init() Foo { return .{ .x = 0 }; }
    fn go(self: *Foo) void { self.run(); }
};

test "a" { _ = add(1, 2); }
`
	check(t, parse(t, "zig", src), `|add|5-8|pub fn add(a: i32, b: i32) i32|Adds.|std.debug>print,>helper,util>run,
Foo|init|12-12|pub fn init() Foo||
Foo|go|13-13|fn go(self: *Foo) void||self>run,
import ./util.zig
`)
}

func TestOCaml(t *testing.T) {
	src := `open Core
module L = List
include Foo.Bar

(* Adds numbers. *)
let add a b = helper a + b

let rec fact n = if n = 0 then 1 else n * fact (n - 1)

let f = fun x -> Printf.printf "%d" x

let v = 3

module M = struct
  let inner x = List.map String.length x |> ignore
end

class point = object
  method move d = ()
end
`
	check(t, parse(t, "ocaml", src), `|add|6-6|let add a b = helper a + b|Adds numbers.|>helper,
|fact|8-8|let rec fact n = if n = 0 then 1 else n * fact (n - 1)||>fact,
|f|10-10|let f = fun x -> Printf.printf "%d" x||Printf>printf,
M|inner|15-15|let inner x = List.map String.length x |> ignore||List>map,>ignore,
point|move|19-19|method move d =||
import Core
import List
import Foo.Bar
`)
}
