package generic

import "testing"

func TestCSharp(t *testing.T) {
	src := `using System;
using static System.Math;
using Foo = Bar.Baz;
namespace App.Core {
  /// <summary>Svc.</summary>
  public class Svc : IBase {
    private Repo repo;
    public Svc() { Init(); }
    /// <summary>Find.</summary>
    public Pet Find(int id) {
      var p = repo.Get(id);
      this.Log(p);
      Helper(id);
      return new Pet(p.Name.Trim());
    }
    public int Len => Compute();
    void Local() { int Inner() { return 1; } }
  }
  public interface IBase { void Run(); }
  public record R(int X) { public int Y() => X; }
  public struct S { public void M() { } }
}
`
	check(t, parse(t, "csharp", src), `Svc|Svc|8-8|public Svc()||>Init,
Svc|Find|10-15|public Pet Find(int id)|Find.|repo>Get,this>Log,>Helper,>Pet,p.Name>Trim,
Svc|Len|16-16|public int Len => Compute();||>Compute,
Svc|Local|17-17|void Local()||
Svc|Inner|17-17|int Inner()||
IBase|Run|19-19|void Run();||
R|Y|20-20|public int Y()||
S|M|21-21|public void M()||
import System
import System.Math
import Bar.Baz
`)
}

func TestPHP(t *testing.T) {
	src := `<?php
namespace App\Svc;
use Foo\Bar;
use Foo\{Baz, Qux as Q};
require_once 'lib/x.php';
include "y.php";
/** Top fn. */
function top($a) { return helper($a) + \Foo\bar(1); }
class Svc extends Base {
  /** Find. */
  public function find($id) {
    $p = $this->repo->get($id);
    self::stat();
    Util::make($p);
    $o = new Pet($p);
    $this->log($p);
    return $o;
  }
}
trait T { function t() {} }
`
	check(t, parse(t, "php", src), `|top|8-8|function top($a)|Top fn.|>helper,?>bar,
Svc|find|11-18|public function find($id)|Find.|this.repo>get,self>stat,Util>make,>Pet,this>log,
T|t|20-20|function t()||
import Foo.Bar
import Foo.Baz
import Foo.Qux
import ./lib/x.php
import ./y.php
`)
}

func TestScala(t *testing.T) {
	src := `package app.core
import scala.collection.mutable
import scala.util.{Try, Success}
import foo.bar._
/** Svc. */
class Svc(repo: Repo) extends Base {
  /** Find. */
  def find(id: Int): Pet = {
    val p = repo.get(id)
    log(p)
    this.stop()
    new Pet(p.name.trim)
  }
}
object Main { def main(args: Array[String]): Unit = { println("x"); Svc.make() } }
trait T { def run(): Unit }
`
	check(t, parse(t, "scala", src), `Svc|find|8-13|def find(id: Int): Pet =|Find.|repo>get,>log,this>stop,>Pet,
Main|main|15-15|def main(args: Array[String]): Unit =||>println,Svc>make,
T|run|16-16|def run(): Unit||
import scala.collection.mutable
import scala.util.Try
import scala.util.Success
import foo.bar.*
`)
}

func TestElixir(t *testing.T) {
	src := `defmodule App.Svc do
  alias App.Repo
  import Enum, only: [map: 2]
  use GenServer
  require Logger

  @doc "Find it."
  def find(id) do
    p = Repo.get(id)
    log(p)
    p |> trim()
  end

  defp log(p), do: IO.puts(p)
  def zero, do: 1
  def g(x) when x > 1, do: x
end
`
	check(t, parse(t, "elixir", src), `App.Svc|find|8-12|def find(id) do|Find it.|Repo>get,>log,>trim,
App.Svc|log|14-14|defp log(p), do: IO.puts(p)||IO>puts,
App.Svc|zero|15-15|def zero, do: 1||
App.Svc|g|16-16|def g(x) when x > 1, do: x||
import App.Repo
import Enum
import GenServer
import Logger
`)
}

func TestHaskell(t *testing.T) {
	src := `module Main where

import qualified Data.Map.Strict as Map

-- | Top doc.
main :: IO ()
main = do
  let m = Map.fromList [(1,2)]
  print (Map.lookup 1 m)
  go 3
  where
    go n = helper n

-- | Second doc
-- more
second :: Int -> Int
second x = x + 1

newtype W = W Int
instance Show W where
  show (W n) = render n
`
	check(t, parse(t, "haskell", src), `|main|7-12|main = do||Map>fromList,Map>lookup,
|go|12-12|go n = helper n||>helper,
|second|17-17|second x = x + 1|Second doc|
W|show|21-21|show (W n) = render n||>render,
import Data.Map.Strict
`)
}

func TestDart(t *testing.T) {
	src := `import 'a.dart' show x;
export 'src/b.dart';
import 'package:foo/bar.dart';
abstract class Base { void abs(); }
class A extends B with M implements I {
  A(this.x) : super() { init(); }
  factory A.f() => A(1);
  /// Runs.
  Future<void> run() async {
    final r = await repo.get(1);
    this.stop();
    super.run();
    helper(r);
    var w = new Foo.bar();
    a.b.c(1);
  }
  set v(int x) { _v = x; }
}
mixin M { void m() {} }
extension E on int { int dbl() => this * 2; }
/// Entry.
void main() { runApp(App()); }
`
	check(t, parse(t, "dart", src), `A|A|6-6|A(this.x) : super()||
A|A|7-7|factory A.f()||>A,
A|run|9-16|Future<void> run()|Runs.|repo>get,this>stop,super>run,>helper,>Foo,a.b>c,
A|v|17-17|set v(int x)||
M|m|19-19|void m()||
E|dbl|20-20|int dbl()||
|main|22-22|void main()|Entry.|>runApp,>App,
import ./a.dart
import ./src/b.dart
import foo/bar.dart
`)
}
