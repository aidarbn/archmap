package js

import (
	"reflect"
	"testing"

	"github.com/aidarbn/archmap/internal/lang"
)

func parse(t *testing.T, path, src string) lang.File {
	t.Helper()
	f, err := New().Parse(path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func byName(f lang.File, name string) *lang.Func {
	for i := range f.Funcs {
		if f.Funcs[i].Name == name {
			return &f.Funcs[i]
		}
	}
	return nil
}

func TestFunctionsAndArrows(t *testing.T) {
	src := `import React, { FC } from 'react'
import './side-effect.css'
const fs = require('fs')
export * from './reexport'
export { a } from "../b"

/**
 * Loads the user.
 * @param id user id
 */
export async function load(
  id: number,
  opts?: Opts
): Promise<User> {
  const u = await api.get(id)
  this.cache.set(id, u)
  const h = () => { inner(u) }
  items.map(x => helper(x)).filter(Boolean)
  load(id - 1)
  new Thing(id)
  return fmt(u)
}

export const Comp: FC<Props> = ({ a, b }) => {
  useEffect(() => { track(a) }, [a])
  return <div onClick={() => click(b)} />
}

const sq = (x: number) => x * x

export const Memo = React.memo(function Memo(props) {
  return render(props)
})

const Fwd = forwardRef<HTMLDivElement, P>((props, ref) => {
  return <div ref={ref}>{wrap(props)}</div>
})

const notFn = 5
const obj = { m() { skip() } }
function sig(a: number): void
function sig(a: any) { return a }
async function lazy() { return import('./lazy-mod') }
`
	f := parse(t, "a.tsx", src)

	var names []string
	for _, fn := range f.Funcs {
		names = append(names, fn.Name)
	}
	want := []string{"load", "Comp", "sq", "Memo", "Fwd", "sig", "lazy"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("funcs = %v, want %v", names, want)
	}

	l := byName(f, "load")
	if l.Start != 11 || l.End != 22 {
		t.Errorf("load range %d-%d", l.Start, l.End)
	}
	if l.Sig != "export async function load( id: number, opts?: Opts ): Promise<User>" {
		t.Errorf("load sig %q", l.Sig)
	}
	if l.Doc != "Loads the user." {
		t.Errorf("load doc %q", l.Doc)
	}
	wantCalls := []lang.Call{mk("get", "api"), mk("set", "this.cache"), mk("inner", ""), mk("map", "items"), mk("helper", ""), mk("filter", "?"), mk("Thing", ""), mk("fmt", "")}
	if !reflect.DeepEqual(l.Calls, wantCalls) {
		t.Errorf("load calls = %v", l.Calls)
	}

	c := byName(f, "Comp")
	if c.Start != 24 || c.End != 27 || c.Sig != "export const Comp: FC<Props> = ({ a, b })" {
		t.Errorf("Comp %d-%d %q", c.Start, c.End, c.Sig)
	}
	if !reflect.DeepEqual(c.Calls, []lang.Call{mk("useEffect", ""), mk("track", ""), mk("click", "")}) {
		t.Errorf("Comp calls = %v", c.Calls)
	}
	if sq := byName(f, "sq"); sq.Start != 29 || sq.End != 29 || sq.Sig != "const sq = (x: number)" {
		t.Errorf("sq %+v", sq)
	}
	if m := byName(f, "Memo"); m.Start != 31 || m.End != 33 {
		t.Errorf("Memo %+v", m)
	}
	if fw := byName(f, "Fwd"); fw.Start != 35 || fw.End != 37 || len(fw.Calls) != 1 || fw.Calls[0].Name != "wrap" {
		t.Errorf("Fwd %+v", fw)
	}
	if sg := byName(f, "sig"); sg.Start != 42 {
		t.Errorf("overload signature must be skipped: %+v", sg)
	}

	wantImps := []string{"react", "./side-effect.css", "fs", "./reexport", "../b", "./lazy-mod"}
	var got []string
	for _, i := range f.Imports {
		got = append(got, i.Path)
	}
	if !reflect.DeepEqual(got, wantImps) {
		t.Errorf("imports = %v", got)
	}
}

func TestClasses(t *testing.T) {
	src := `export default class Store extends Base {
  state = 1
  // Handles click.
  onClick = async (e) => {
    this.save(e)
  }
  constructor() { super() }
  @Memo()
  private get total(): number {
    return this.items.reduce(sum, 0) + super.base()
  }
  static make() { return Store.build() }
  abstract nope(): void
}
`
	f := parse(t, "s.ts", src)
	if f.DefaultExport != "Store" {
		t.Errorf("default = %q", f.DefaultExport)
	}
	if len(f.Funcs) != 3 {
		t.Fatalf("funcs = %+v", f.Funcs)
	}
	oc := byName(f, "onClick")
	if oc.Cls != "Store" || oc.Start != 4 || oc.End != 6 || oc.Doc != "Handles click." {
		t.Errorf("onClick %+v", oc)
	}
	if oc.Sig != "onClick = async (e)" {
		t.Errorf("onClick sig %q", oc.Sig)
	}
	tot := byName(f, "total")
	if tot.Start != 9 || tot.TStart != 8 || !reflect.DeepEqual(tot.Decorators, []string{"Memo()"}) {
		t.Errorf("total %+v", tot)
	}
	if !reflect.DeepEqual(tot.Calls, []lang.Call{mk("reduce", "this.items"), mk("base", "super")}) {
		t.Errorf("total calls %v", tot.Calls)
	}
	mk := byName(f, "make")
	if mk.Calls[0] != (lang.Call{Name: "build", Recv: "Store"}) {
		t.Errorf("make calls %v", mk.Calls)
	}
}

func TestDefaultExports(t *testing.T) {
	cases := map[string]string{
		"export default function Page() { return null }":             "Page",
		"const P = () => null\nexport default P":                     "P",
		"const P = () => null\nexport default memo(P)":               "P",
		"export default () => null":                                  "",
		"export default async function () {}":                        "",
		"export const x = 1":                                         "",
		"export default React.memo(function Foo() { return 1 })":     "",
		"export default function Foo() {}\nexport const y = () => 1": "Foo",
	}
	for src, want := range cases {
		if got := parse(t, "p.jsx", src).DefaultExport; got != want {
			t.Errorf("%q: default = %q, want %q", src, got, want)
		}
	}
}

func TestSyntaxErrorDoesNotPanic(t *testing.T) {
	f := parse(t, "bad.js", "function ok() { return a() }\nfunction broken( {{{\n")
	if byName(f, "ok") == nil {
		t.Errorf("expected ok to be recovered: %+v", f.Funcs)
	}
	parse(t, "empty.mjs", "")
}

func TestPlainJS(t *testing.T) {
	f := parse(t, "x.mjs", "export function a() { b?.c(); d!.e() }\nconst b = function() { return a() }\n")
	if len(f.Funcs) != 2 {
		t.Fatalf("%+v", f.Funcs)
	}
	if !reflect.DeepEqual(f.Funcs[0].Calls, []lang.Call{mk("c", "b"), mk("e", "d")}) {
		t.Errorf("calls %v", f.Funcs[0].Calls)
	}
	if !reflect.DeepEqual(f.Funcs[1].Calls, []lang.Call{mk("a", "")}) {
		t.Errorf("calls %v", f.Funcs[1].Calls)
	}
}

func mk(name, recv string) lang.Call { return lang.Call{Name: name, Recv: recv} }
