package evaluator

import (
	"bytes"
	"io"
	"testing"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
)

// parseForBench parses src once under the 2.6 target. Benchmarks reuse
// the resulting program across iterations so the measured cost is
// purely evaluation, not lex/parse.
func parseForBench(b *testing.B, src string) *ast.Program {
	b.Helper()
	target := token.MustParseVersion("2.6")
	prog, err := parser.ParseFile("bench.rb", []byte(src), 0, parser.WithVersion(target))
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	return prog
}

// newBenchEnv returns a fresh main env with stdout discarded -- puts /
// p output in the snippet shouldn't dominate the timing.
func newBenchEnv() *object.Environment {
	target := token.MustParseVersion("2.6")
	return object.NewMainEnvironment(
		object.WithVersion(target),
		object.WithStdout(io.Discard),
	)
}

// runEvalBench is the boilerplate harness: parse once, then re-eval
// against a fresh env each iteration so global state (top-level
// methods, constants) doesn't leak between runs.
func runEvalBench(b *testing.B, src string) {
	prog := parseForBench(b, src)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env := newBenchEnv()
		if _, err := Eval(prog, env); err != nil {
			b.Fatalf("eval: %v", err)
		}
	}
}

// BenchmarkEvalArithmeticLoop hammers the integer-infix fast path and
// the Integer#times block dispatch on every iteration.
func BenchmarkEvalArithmeticLoop(b *testing.B) {
	runEvalBench(b, `
acc = 0
100.times do |i|
  acc = acc + i * 2 - 1
end
acc
`)
}

// BenchmarkEvalRecursiveFib exercises method-call dispatch and frame
// setup on a classic recursive workload.
func BenchmarkEvalRecursiveFib(b *testing.B) {
	runEvalBench(b, `
def fib(n)
  return n if n < 2
  fib(n - 1) + fib(n - 2)
end
fib(15)
`)
}

// BenchmarkEvalArrayMap covers Array#map block dispatch, block-param
// binding, and the implicit-self / Send paths on the resulting array.
func BenchmarkEvalArrayMap(b *testing.B) {
	runEvalBench(b, `
xs = (1..50).to_a
ys = xs.map { |x| x * x + 1 }
ys.sum
`)
}

// BenchmarkEvalHashConstructAndAccess walks the Hash literal builder,
// hash insert, hash lookup, and the iteration path via each_pair.
func BenchmarkEvalHashConstructAndAccess(b *testing.B) {
	runEvalBench(b, `
h = { a: 1, b: 2, c: 3, d: 4, e: 5 }
total = 0
h.each_pair { |_k, v| total += v }
total + h[:c]
`)
}

// BenchmarkEvalStringInterpolation exercises stringification + writer
// fan-in along the puts kernel path.
func BenchmarkEvalStringInterpolation(b *testing.B) {
	runEvalBench(b, `
buf = ""
20.times do |i|
  buf = buf + "i=#{i};"
end
buf.length
`)
}

// BenchmarkEvalUserClassDispatch covers user-class instantiation,
// instance variable get/set, and method-call dispatch through Send on
// the migrated class chain.
func BenchmarkEvalUserClassDispatch(b *testing.B) {
	runEvalBench(b, `
class Counter
  def initialize
    @n = 0
  end
  def bump(by)
    @n += by
    self
  end
  def value
    @n
  end
end
c = Counter.new
50.times { |i| c.bump(i) }
c.value
`)
}

// BenchmarkEvalComparableFromSpaceship measures the Comparable
// derivation path -- Object#<, Object#<= etc. now living on
// ObjectClass, dispatched via the user-defined <=>.
func BenchmarkEvalComparableFromSpaceship(b *testing.B) {
	runEvalBench(b, `
class Money
  def initialize(n); @n = n; end
  def <=>(other); @n <=> other.n; end
  def n; @n; end
end
a = Money.new(10)
b = Money.new(20)
20.times do
  a < b
  a <= b
  a == b
  b > a
  a.between?(Money.new(0), b)
end
`)
}

// BenchmarkEvalEnumerableDerivations measures the Enumerable
// derivation path -- to_a, count, include?, min, max, sort -- routed
// through ObjectClass methods that call back into a user each.
func BenchmarkEvalEnumerableDerivations(b *testing.B) {
	runEvalBench(b, `
class Box
  include Enumerable
  def initialize(*xs); @xs = xs; end
  def each(&blk); @xs.each(&blk); end
end
box = Box.new(5, 3, 1, 4, 2)
box.to_a
box.count
box.include?(3)
box.min
box.max
box.sort
`)
}

// BenchmarkEvalRangeIteration walks the Range#each / Range#map paths.
func BenchmarkEvalRangeIteration(b *testing.B) {
	runEvalBench(b, `
sum = 0
(1..100).each { |i| sum += i }
sum + (1..50).map { |i| i * 2 }.sum
`)
}

// BenchmarkEvalSendDispatch hits the Object#send universal-method path
// repeatedly, measuring the symbol/method-name resolution overhead.
func BenchmarkEvalSendDispatch(b *testing.B) {
	runEvalBench(b, `
arr = [1, 2, 3, 4, 5]
total = 0
50.times do
  total += arr.send(:length)
  total += arr.send(:first)
end
total
`)
}

// BenchmarkEvalSymbolToProcMap exercises the &:sym shorthand and
// Symbol#to_proc dispatch onto Array#map.
func BenchmarkEvalSymbolToProcMap(b *testing.B) {
	runEvalBench(b, `
words = ["alpha", "beta", "gamma", "delta", "epsilon"]
words.map(&:upcase).map(&:length).sum
`)
}

// BenchmarkEvalParseAndEval is the end-to-end shape: parse + eval on
// every iteration, for snippets a REPL or test harness would hit.
func BenchmarkEvalParseAndEval(b *testing.B) {
	src := `
def add(a, b); a + b; end
result = 0
30.times { |i| result = add(result, i) }
result
`
	target := token.MustParseVersion("2.6")
	buf := []byte(src)
	b.SetBytes(int64(len(buf)))
	b.ReportAllocs()
	b.ResetTimer()
	var out bytes.Buffer
	for i := 0; i < b.N; i++ {
		prog, err := parser.ParseFile("bench.rb", buf, 0, parser.WithVersion(target))
		if err != nil {
			b.Fatalf("parse: %v", err)
		}
		out.Reset()
		env := object.NewMainEnvironment(object.WithVersion(target), object.WithStdout(&out))
		if _, err := Eval(prog, env); err != nil {
			b.Fatalf("eval: %v", err)
		}
	}
}
