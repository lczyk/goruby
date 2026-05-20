package evaluator

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/goruby/object"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
)

// run is a convenience that parses src under the 2.6 target version,
// evaluates it under a fresh env, and returns captured stdout. Fails
// the test on parse / eval errors.
func run(t *testing.T, src string) string {
	t.Helper()
	target := token.MustParseVersion("2.6")
	prog, err := parser.ParseFile("<test>", []byte(src), 0, parser.WithVersion(target))
	assert.NoError(t, err, "parse")

	var stdout bytes.Buffer
	env := object.NewMainEnvironment(object.WithVersion(target), object.WithStdout(&stdout))
	_, err = Eval(prog, env)
	assert.NoError(t, err, "eval")
	return stdout.String()
}

// runExpect parses + evaluates src and asserts an exact stdout match.
func runExpect(t *testing.T, src, want string) {
	t.Helper()
	got := run(t, src)
	assert.Equal(t, want, got, "stdout for %q", src)
}

func TestIsConstantName(t *testing.T) {
	assert.That(t, isConstantName("Foo"), "Foo is constant")
	assert.That(t, isConstantName("X"), "X is constant")
	assert.That(t, !isConstantName("foo"), "foo is not constant")
	assert.That(t, !isConstantName(""), "empty is not constant")
	assert.That(t, !isConstantName("_X"), "_X is not constant (leading underscore)")
}

func TestDecodeStringEscapesDoubleQuoted(t *testing.T) {
	assert.Equal(t, "ab\ncd", decodeStringEscapes(`ab\ncd`, false))
	assert.Equal(t, "\t\r\\\"", decodeStringEscapes(`\t\r\\\"`, false))
	assert.Equal(t, "no escapes", decodeStringEscapes("no escapes", false))
	// Unknown escape in dq drops the backslash.
	assert.Equal(t, "q", decodeStringEscapes(`\q`, false))
}

func TestDecodeStringEscapesSingleQuoted(t *testing.T) {
	// Single-quoted only honours \\ and \'.
	assert.Equal(t, `ab\ncd`, decodeStringEscapes(`ab\ncd`, true))
	assert.Equal(t, `\`, decodeStringEscapes(`\\`, true))
	assert.Equal(t, `'`, decodeStringEscapes(`\'`, true))
}

func TestRubyEqualByType(t *testing.T) {
	assert.That(t, rubyEqual(object.NewInteger(3), object.NewInteger(3)), "Integer == Integer")
	assert.That(t, !rubyEqual(object.NewInteger(3), object.NewInteger(4)), "Integer != Integer")
	assert.That(t, !rubyEqual(object.NewInteger(3), object.NewString("3")), "type mismatch is unequal")
	assert.That(t, rubyEqual(object.NIL, object.NIL), "nil == nil")
}

func TestTruthy(t *testing.T) {
	assert.That(t, !truthy(object.NIL), "nil is falsy")
	assert.That(t, !truthy(object.FALSE), "false is falsy")
	assert.That(t, truthy(object.TRUE), "true is truthy")
	assert.That(t, truthy(object.NewInteger(0)), "0 is truthy in ruby")
	assert.That(t, truthy(object.NewString("")), "empty string is truthy in ruby")
}

func TestCompareObjectsIntegers(t *testing.T) {
	c, ok := compareObjects(object.NewInteger(1), object.NewInteger(2))
	assert.That(t, ok, "Integer pair comparable")
	assert.Equal(t, -1, c)

	c, ok = compareObjects(object.NewInteger(5), object.NewInteger(5))
	assert.That(t, ok, "Integer eq pair comparable")
	assert.Equal(t, 0, c)

	_, ok = compareObjects(object.NewInteger(1), object.NewString("x"))
	assert.That(t, !ok, "cross-type pair not comparable")
}

// End-to-end smoke tests covering each major dispatch branch. The full
// corpus harness exercises behaviour at scale; these guard the
// fast-path tree-walker against silent breakage in single-purpose tests
// without booting the corpus runner.

func TestEvalArithmetic(t *testing.T) {
	runExpect(t, "puts 1 + 2 * 3", "7\n")
	runExpect(t, "puts (1 + 2) * 3", "9\n")
	runExpect(t, "puts 10 / 3", "3\n")
	runExpect(t, "puts -7 / 2", "-4\n") // ruby floor division
	runExpect(t, "puts 2 ** 8", "256\n")
}

func TestEvalShortCircuit(t *testing.T) {
	// `||` returns the operand that decided the result, not a bool.
	runExpect(t, `puts (nil || "fallback")`, "fallback\n")
	runExpect(t, `puts (false && raise("boom"))`, "false\n")
}

func TestEvalConditional(t *testing.T) {
	runExpect(t, `puts "yes" if 1 < 2`, "yes\n")
	runExpect(t, `puts "no" unless 1 < 2`, "")
	runExpect(t, `puts(1 < 2 ? "a" : "b")`, "a\n")
}

func TestEvalLoop(t *testing.T) {
	src := `i = 0
while i < 3
  puts i
  i = i + 1
end`
	runExpect(t, src, "0\n1\n2\n")
}

func TestEvalDefAndCall(t *testing.T) {
	src := `def add(a, b = 10)
  a + b
end
puts add(1)
puts add(2, 3)`
	runExpect(t, src, "11\n5\n")
}

func TestEvalReturnUnwindsToCallFrame(t *testing.T) {
	src := `def f
  return 99
  100  # unreachable
end
puts f`
	runExpect(t, src, "99\n")
}

func TestEvalClassAndInstance(t *testing.T) {
	src := `class C
  def initialize(x)
    @x = x
  end
  def x
    @x
  end
end
c = C.new(42)
puts c.x
puts c.class
puts c.is_a?(C)`
	runExpect(t, src, "42\nC\ntrue\n")
}

func TestEvalAttrAccessor(t *testing.T) {
	src := `class Point
  attr_accessor :x
  def initialize(x)
    @x = x
  end
end
p = Point.new(3)
p.x = 10
puts p.x`
	runExpect(t, src, "10\n")
}

func TestEvalInheritanceAndSuper(t *testing.T) {
	src := `class A
  def name
    "a"
  end
end
class B < A
  def name
    super + "b"
  end
end
puts B.new.name`
	runExpect(t, src, "ab\n")
}

func TestEvalBlockAndYield(t *testing.T) {
	src := `def twice
  yield
  yield
end
twice { puts "x" }`
	runExpect(t, src, "x\nx\n")
}

func TestEvalArrayBlockMethods(t *testing.T) {
	runExpect(t, "p [1, 2, 3].map { |x| x * x }", "[1, 4, 9]\n")
	runExpect(t, "puts [1, 2, 3].reduce(0) { |s, x| s + x }", "6\n")
}

func TestEvalLambda(t *testing.T) {
	src := `f = ->(a, b) { a + b }
puts f.call(2, 3)
puts f.(4, 5)`
	runExpect(t, src, "5\n9\n")
}

func TestEvalCapturedBlockArg(t *testing.T) {
	src := `def run(&blk)
  blk.call(7)
end
puts run { |n| n + 1 }`
	runExpect(t, src, "8\n")
}

func TestEvalRaiseRescue(t *testing.T) {
	src := `begin
  raise "boom"
rescue => e
  puts e.message
end`
	runExpect(t, src, "boom\n")
}

func TestEvalEnsureAlwaysRuns(t *testing.T) {
	src := `ran = false
begin
  raise "x"
rescue
  # swallow
ensure
  ran = true
end
puts ran`
	runExpect(t, src, "true\n")
}

func TestEvalZeroDivisionRaises(t *testing.T) {
	src := `begin
  1 / 0
rescue ZeroDivisionError => e
  puts e.message
end`
	runExpect(t, src, "divided by 0\n")
}

func TestEvalSafeNavigation(t *testing.T) {
	src := `s = nil
puts s&.length
puts "abc"&.length`
	runExpect(t, src, "\n3\n")
}

func TestEvalModuleAndInclude(t *testing.T) {
	src := `module M
  def tag
    "m"
  end
end
class C
  include M
end
puts C.new.tag
puts C.new.is_a?(M)`
	runExpect(t, src, "m\ntrue\n")
}

func TestEvalScopedConstant(t *testing.T) {
	src := `module Cfg
  V = 42
end
puts Cfg::V`
	runExpect(t, src, "42\n")
}

func TestEvalStringInterpolation(t *testing.T) {
	src := `name = "world"
puts "hello, #{name}!"
puts "1+2=#{1+2}"`
	runExpect(t, src, "hello, world!\n1+2=3\n")
}

func TestEvalRangeIteration(t *testing.T) {
	runExpect(t, "(1..3).each { |i| puts i }", "1\n2\n3\n")
	runExpect(t, "(1...3).each { |i| puts i }", "1\n2\n")
	runExpect(t, "p (1..4).to_a", "[1, 2, 3, 4]\n")
	runExpect(t, "p (1...4).to_a", "[1, 2, 3]\n")
	runExpect(t, "puts (1..5).size", "5\n")
	runExpect(t, "puts (1..5).include?(3)", "true\n")
	runExpect(t, "puts (1...5).include?(5)", "false\n")
	runExpect(t, "p (1..3).map { |i| i * 10 }", "[10, 20, 30]\n")
}

func TestEvalForIn(t *testing.T) {
	runExpect(t, "for x in [1,2,3]\n  puts x\nend", "1\n2\n3\n")
	runExpect(t, "for x in (1..3)\n  puts x\nend", "1\n2\n3\n")
	src := `sum = 0
for x in [10, 20, 30]
  sum = sum + x
end
puts sum`
	runExpect(t, src, "60\n")
}

func TestEvalHashEach(t *testing.T) {
	runExpect(t, `{ a: 1, b: 2 }.each { |k, v| puts "#{k}=#{v}" }`, "a=1\nb=2\n")
}

func TestEvalArrayAggregates(t *testing.T) {
	runExpect(t, `puts [3, 1, 4, 1, 5].min`, "1\n")
	runExpect(t, `puts [3, 1, 4, 1, 5].max`, "5\n")
	runExpect(t, `puts [1, 2, 3, 4].sum`, "10\n")
	runExpect(t, `p [1, 2, 2, 3, 3, 3].uniq`, "[1, 2, 3]\n")
	runExpect(t, `puts [1, 2, 3].join("-")`, "1-2-3\n")
}

func TestEvalArrayFlatten(t *testing.T) {
	runExpect(t, `p [1, [2, [3, 4]], 5].flatten`, "[1, 2, 3, 4, 5]\n")
}

func TestEvalStringSplitAndChars(t *testing.T) {
	runExpect(t, `p "a,b,c".split(",")`, `["a", "b", "c"]`+"\n")
	runExpect(t, `p "abc".chars`, `["a", "b", "c"]`+"\n")
	runExpect(t, `puts "hello".start_with?("he")`, "true\n")
	runExpect(t, `puts "hello".end_with?("zz", "lo")`, "true\n")
	runExpect(t, `puts "line\n".chomp`, "line\n")
	runExpect(t, `puts "42abc".to_i`, "42\n")
}

func TestEvalIntegerTimes(t *testing.T) {
	src := `sum = 0
3.times { |i| sum = sum + i }
puts sum`
	runExpect(t, src, "3\n") // 0+1+2
}

func TestEvalKwargsAsTrailingHash(t *testing.T) {
	// Pre-ruby-3 calling convention: `f(name: "x")` passes `{name: "x"}`
	// as the trailing positional arg when the callee declares no
	// keyword params. Single key keeps the test deterministic across
	// Go's map iteration order.
	src := `def f(data); data[:name]; end
puts f(name: "x")`
	runExpect(t, src, "x\n")
}

func TestEvalSchemaValidator(t *testing.T) {
	src := `class S
  def initialize; @r = []; end
  def required(k, t); @r << [:r, k, t]; self; end
  def validate(d)
    out = []
    @r.each do |kind, k, t|
      if d.key?(k)
        out << "#{k}: bad" unless d[k].is_a?(t)
      else
        out << "missing #{k}"
      end
    end
    out
  end
end
s = S.new.required(:age, Integer)
p s.validate(age: 30)
p s.validate(age: "x")
p s.validate(name: "y")`
	runExpect(t, src, "[]\n[\"age: bad\"]\n[\"missing age\"]\n")
}

func TestEvalMethodMissingSettings(t *testing.T) {
	src := `class Settings
  def initialize(h); @h = h; end
  def method_missing(name, *args)
    return @h[name] if @h.key?(name)
    super
  end
  def respond_to_missing?(name, include_private = false)
    @h.key?(name) || super
  end
end
s = Settings.new(host: "localhost", port: 8080)
puts s.host
puts s.port`
	runExpect(t, src, "localhost\n8080\n")
}

func TestEvalBrainfuckHelloWorld(t *testing.T) {
	// End-to-end: a small Brainfuck interpreter exercising
	// Array.new(n, fill), Hash lookups, while-loop, case-when with
	// String, String << Integer.chr, Array#[] / []=, etc.
	src := `def bf(src)
  tape = Array.new(64, 0)
  ptr = 0; ip = 0; out = ""
  loops = {}; stack = []
  src.chars.each_with_index do |c, i|
    if c == "["
      stack << i
    elsif c == "]"
      j = stack.pop; loops[i] = j; loops[j] = i
    end
  end
  while ip < src.length
    c = src[ip]
    case c
    when "+" then tape[ptr] += 1
    when "-" then tape[ptr] -= 1
    when ">" then ptr += 1
    when "<" then ptr -= 1
    when "." then out << tape[ptr].chr
    when "[" then ip = loops[ip] if tape[ptr] == 0
    when "]" then ip = loops[ip] if tape[ptr] != 0
    end
    ip += 1
  end
  out
end
puts bf("++++++++[>++++[>++>+++>+++>+<<<<-]>+>+>->>+[<]<-]>>.>---.+++++++..+++.>>.<-.<.+++.------.--------.>>+.>++.")`
	runExpect(t, src, "Hello World!\n")
}

func TestEvalArrayMinMax(t *testing.T) {
	runExpect(t, `p [3, 1, 4, 1, 5].minmax`, "[1, 5]\n")
	runExpect(t, `p [].minmax`, "[nil, nil]\n")
	runExpect(t, `p [3, 1, 4, 1, 5].minmax_by { |x| -x }`, "[5, 1]\n")
}

func TestEvalArrayPredicatesNoBlock(t *testing.T) {
	runExpect(t, `puts [1, 2, 3].all?`, "true\n")
	runExpect(t, `puts [1, nil, 3].all?`, "false\n")
	runExpect(t, `puts [].all?`, "true\n")
	runExpect(t, `puts [nil, false].any?`, "false\n")
	runExpect(t, `puts [nil, 1].any?`, "true\n")
	runExpect(t, `puts [nil, false].none?`, "true\n")
	runExpect(t, `puts [1, nil].one?`, "true\n")
	runExpect(t, `puts [1, 2].one?`, "false\n")
}

func TestEvalHashSumMinMaxBy(t *testing.T) {
	runExpect(t, `puts({a: 1, b: 2, c: 3}.sum { |_, v| v })`, "6\n")
	runExpect(t, `p({a: 1, b: 5, c: 3}.min_by { |_, v| v })`, "[:a, 1]\n")
	runExpect(t, `p({a: 1, b: 5, c: 3}.max_by { |_, v| v })`, "[:b, 5]\n")
}

func TestEvalIntegerToFloat(t *testing.T) {
	runExpect(t, `puts 3.to_f`, "3.0\n")
	runExpect(t, `puts 10.to_f / 3`, "3.3333333333333335\n")
}

func TestEvalArrayFlatMap(t *testing.T) {
	runExpect(t, `p [[1, 2], [3, 4]].flat_map { |a| a.map { |x| x * 10 } }`, "[10, 20, 30, 40]\n")
	runExpect(t, `p [1, 2, 3].flat_map { |x| [x, -x] }`, "[1, -1, 2, -2, 3, -3]\n")
}

func TestEvalHashMergeWithBlock(t *testing.T) {
	src := `a = {x: 1, y: 2}
b = {y: 20, z: 30}
p a.merge(b) { |k, v1, v2| v1 + v2 }`
	runExpect(t, src, "{:x=>1, :y=>22, :z=>30}\n")
}

func TestEvalArrayZipWithBlock(t *testing.T) {
	src := `out = []
[10, 20, 30].zip([100, 200, 300]) { |a, b| out << a + b }
p out`
	runExpect(t, src, "[110, 220, 330]\n")
}

func TestEvalArrayNewWithBlock(t *testing.T) {
	runExpect(t, `p Array.new(5) { |i| i * i }`, "[0, 1, 4, 9, 16]\n")
	runExpect(t, `p Array.new(3) { "x" }`, `["x", "x", "x"]`+"\n")
}

func TestEvalTTLStore(t *testing.T) {
	// Cross-cutting: classes + kwarg + Hash w/ Symbol keys + each-
	// with-callback + block-yielding each + expiry logic + Hash
	// deletion mid-traversal-safe usage.
	src := `class S
  def initialize(now = 0); @d = {}; @now = now; end
  def now=(t); @now = t; end
  def set(k, v, ttl: nil); @d[k] = { v: v, exp: ttl ? @now + ttl : nil }; self; end
  def get(k)
    e = @d[k]
    return nil unless e
    if e[:exp] && e[:exp] <= @now; @d.delete(k); return nil; end
    e[:v]
  end
  def keys; @d.keys; end
end
s = S.new(0)
s.set(:a, 1)
s.set(:b, 2, ttl: 5)
s.now = 10
puts s.get(:a)
puts s.get(:b).inspect
p s.keys`
	runExpect(t, src, "1\nnil\n[:a]\n")
}

func TestEvalArraySortWithBlock(t *testing.T) {
	runExpect(t, `p [3, 1, 4, 1, 5].sort { |a, b| b <=> a }`, "[5, 4, 3, 1, 1]\n")
}

func TestEvalRangeEachWithObject(t *testing.T) {
	runExpect(t, `p (1..3).each_with_object({}) { |x, h| h[x] = x * x }`, "{1=>1, 2=>4, 3=>9}\n")
}

func TestEvalCaseRegexPattern(t *testing.T) {
	src := `def f(s)
  case s
  when /^\d+$/ then "num"
  when /^\w+$/ then "word"
  else "other"
  end
end
puts f("123")
puts f("abc")
puts f("!!")`
	runExpect(t, src, "num\nword\nother\n")
}

func TestEvalRot13(t *testing.T) {
	src := `def rot13(s)
  s.chars.map do |c|
    case c
    when /[a-z]/ then ((c.ord - "a".ord + 13) % 26 + "a".ord).chr
    when /[A-Z]/ then ((c.ord - "A".ord + 13) % 26 + "A".ord).chr
    else c
    end
  end.join
end
puts rot13("Hello, World!")`
	runExpect(t, src, "Uryyb, Jbeyq!\n")
}

func TestEvalStringCountAndBytes(t *testing.T) {
	runExpect(t, `puts "Hello".count("l")`, "2\n")
	runExpect(t, `puts "Hello".count("lo")`, "3\n")
	runExpect(t, `p "abc".bytes`, "[97, 98, 99]\n")
	runExpect(t, `puts "abc".bytesize`, "3\n")
}

func TestEvalKwargsRest(t *testing.T) {
	src := `def make(**opts)
  opts.size
end
puts make(a: 1, b: 2, c: 3)`
	runExpect(t, src, "3\n")
}

func TestEvalClassVar(t *testing.T) {
	src := `class C
  def self.inc; @@n ||= 0; @@n += 1; end
  def self.count; @@n; end
end
C.inc; C.inc; C.inc
puts C.count`
	runExpect(t, src, "3\n")
}

func TestEvalClassVarSharedWithSubclass(t *testing.T) {
	src := `class A
  @@x = 0
  def self.bump; @@x += 1; end
  def self.x; @@x; end
end
class B < A; end
A.bump; B.bump; A.bump
puts A.x
puts B.x`
	runExpect(t, src, "3\n3\n")
}

func TestEvalInstanceVariableSetGet(t *testing.T) {
	src := `class C
  def initialize; instance_variable_set(:@x, 42); end
  def x; instance_variable_get(:@x); end
end
puts C.new.x`
	runExpect(t, src, "42\n")
}

func TestEvalConfigDSL(t *testing.T) {
	// Class-level macro DSL exercising: class-method dispatch via
	// implicit self, **kwargs rest, attr_accessor inside a class
	// method, class vars, instance_variable_set, inheritance.
	src := `class Config
  def self.field(name, default: nil)
    attr_accessor name
    @@defaults ||= {}
    @@defaults[name] = default
  end
  def self.defaults; @@defaults || {}; end
  def initialize(**opts)
    self.class.defaults.each { |k, v| instance_variable_set("@#{k}", opts[k] || v) }
  end
end
class App < Config
  field :host, default: "localhost"
  field :port, default: 8080
end
c = App.new(port: 9000)
puts c.host
puts c.port`
	runExpect(t, src, "localhost\n9000\n")
}

func TestEvalFloatStep(t *testing.T) {
	src := `out = []
0.0.step(1.0, 0.25) { |v| out << v }
p out`
	runExpect(t, src, "[0.0, 0.25, 0.5, 0.75, 1.0]\n")
}

func TestEvalComparableClampOnUserType(t *testing.T) {
	src := `class T
  include Comparable
  attr_reader :v
  def initialize(v); @v = v; end
  def <=>(o); @v <=> o.v; end
end
puts T.new(5).clamp(T.new(0), T.new(10)).v
puts T.new(-1).clamp(T.new(0), T.new(10)).v
puts T.new(99).clamp(T.new(0), T.new(10)).v`
	runExpect(t, src, "5\n0\n10\n")
}

func TestEvalChunkAndSliceWhile(t *testing.T) {
	runExpect(t, `p [1, 2, 3, 5, 6, 7, 10].chunk_while { |a, b| b - a == 1 }`, "[[1, 2, 3], [5, 6, 7], [10]]\n")
	runExpect(t, `p [1, 2, 4, 9, 10, 11, 15].slice_when { |a, b| b - a > 1 }`, "[[1, 2], [4], [9, 10, 11], [15]]\n")
}

func TestEvalRecursiveDescentParser(t *testing.T) {
	// End-to-end: a tiny JSON-ish parser exercising classes,
	// recursive instance methods, regex, string indexing, while,
	// raise, case-when, hash + array literals + mutation.
	src := `class P
  def initialize(s); @s = s; @pos = 0; end
  def parse
    skip
    v = val
    v
  end
  private
  def skip; @pos += 1 while @pos < @s.length && @s[@pos] =~ /\s/; end
  def val
    skip
    c = @s[@pos]
    case
    when c == "[" then arr
    when c == "{" then obj
    when c == "\"" then str
    when c =~ /[0-9-]/ then num
    when @s[@pos, 4] == "true" then @pos += 4; true
    when @s[@pos, 5] == "false" then @pos += 5; false
    when @s[@pos, 4] == "null" then @pos += 4; nil
    end
  end
  def arr
    @pos += 1; out = []; skip
    while @s[@pos] != "]"
      out << val; skip
      (@pos += 1; skip) if @s[@pos] == ","
    end
    @pos += 1; out
  end
  def obj
    @pos += 1; out = {}; skip
    while @s[@pos] != "}"
      k = str; skip; @pos += 1; out[k] = val; skip
      (@pos += 1; skip) if @s[@pos] == ","
    end
    @pos += 1; out
  end
  def str
    @pos += 1; start = @pos
    @pos += 1 while @s[@pos] != "\""
    s = @s[start, @pos - start]; @pos += 1; s
  end
  def num
    start = @pos
    @pos += 1 while @pos < @s.length && @s[@pos] =~ /[0-9.-]/
    @s[start, @pos - start].to_i
  end
end
p P.new('[1, 2, 3, true, false, null]').parse
p P.new('{"a": 1, "b": [2, 3]}').parse`
	runExpect(t, src, "[1, 2, 3, true, false, nil]\n"+`{"a"=>1, "b"=>[2, 3]}`+"\n")
}

func TestEvalEnumerableMaterialised(t *testing.T) {
	// Methods that fall through Enumerable to Array dispatch.
	src := `class L
  include Enumerable
  def initialize(*xs); @xs = xs; end
  def each; @xs.each { |x| yield x }; end
end
l = L.new(3, 1, 4, 1, 5)
puts l.sort.inspect
puts l.uniq.inspect
puts l.reverse.inspect
puts l.sum
puts l.reduce(:+)
puts l.join(",")`
	runExpect(t, src, "[1, 1, 3, 4, 5]\n[3, 1, 4, 5]\n[5, 1, 4, 1, 3]\n14\n14\n3,1,4,1,5\n")
}

func TestEvalStringRegexMethods(t *testing.T) {
	runExpect(t, `puts "hello".match?(/lo/)`, "true\n")
	runExpect(t, `puts "hello".gsub(/l/, "L")`, "heLLo\n")
	runExpect(t, `puts "abc-def".sub(/-/, "_")`, "abc_def\n")
	runExpect(t, `p "hello world".scan(/\w+/)`, `["hello", "world"]`+"\n")
	runExpect(t, `p "a1b22".scan(/(\w)(\d+)/)`, `[["a", "1"], ["b", "22"]]`+"\n")
}

func TestEvalRegexMatch(t *testing.T) {
	runExpect(t, `puts "hello world" =~ /world/`, "6\n")
	runExpect(t, `puts ("hello" =~ /xyz/).inspect`, "nil\n")
	runExpect(t, `puts "abc" !~ /xyz/`, "true\n")
	runExpect(t, `puts "abc" !~ /b/`, "false\n")
}

func TestEvalRegexCaseInsensitive(t *testing.T) {
	runExpect(t, `puts "HELLO" =~ /hello/i`, "0\n")
}

func TestEvalIntegerModuloFdiv(t *testing.T) {
	runExpect(t, `puts 7.modulo(3)`, "1\n")
	runExpect(t, `puts (-7).modulo(3)`, "2\n")
	runExpect(t, `puts 7.fdiv(2)`, "3.5\n")
}

func TestEvalFloatRoundDigits(t *testing.T) {
	runExpect(t, `puts 3.14159.round(2)`, "3.14\n")
	runExpect(t, `puts 1.55.round(1)`, "1.6\n")
}

func TestEvalFloatClampInfinityNan(t *testing.T) {
	runExpect(t, `puts 5.5.clamp(1.0, 10.0)`, "5.5\n")
	runExpect(t, `puts Float::INFINITY`, "Infinity\n")
	runExpect(t, `puts (-Float::INFINITY)`, "-Infinity\n")
	runExpect(t, `puts Float::NAN`, "NaN\n")
}

func TestEvalIntegerClampRange(t *testing.T) {
	runExpect(t, `puts 5.clamp(1..10)`, "5\n")
	runExpect(t, `puts 100.clamp(1..10)`, "10\n")
	runExpect(t, `puts (-3).clamp(1..10)`, "1\n")
}

func TestEvalItself(t *testing.T) {
	runExpect(t, `p [1, 2, 3].select(&:itself)`, "[1, 2, 3]\n")
	runExpect(t, `p [nil, false, 0].select(&:itself)`, "[0]\n")
}

func TestEvalHashSortMinMax(t *testing.T) {
	runExpect(t, `p({c: 3, a: 1, b: 2}.sort)`, "[[:a, 1], [:b, 2], [:c, 3]]\n")
	runExpect(t, `p({c: 3, a: 1, b: 2}.min)`, "[:a, 1]\n")
	runExpect(t, `p({c: 3, a: 1, b: 2}.max)`, "[:c, 3]\n")
}

func TestEvalDefBodyRescue(t *testing.T) {
	src := `def parse_int(s)
  Integer(s)
rescue ArgumentError => e
  "err: #{e.message}"
end
puts parse_int("42")
puts parse_int("nope")`
	runExpect(t, src, `42`+"\n"+`err: invalid value for Integer(): "nope"`+"\n")
}

func TestEvalImplicitSelfNewInClassMethod(t *testing.T) {
	// `new(v)` inside a `def self.foo` must dispatch to self.class.new.
	src := `class C
  attr_reader :v
  def initialize(v); @v = v; end
  def self.make(v); new(v); end
end
puts C.make(42).v`
	runExpect(t, src, "42\n")
}

func TestEvalStringRange(t *testing.T) {
	runExpect(t, `p ("a".."e").to_a`, `["a", "b", "c", "d", "e"]`+"\n")
	runExpect(t, `p ("A".."C").to_a`, `["A", "B", "C"]`+"\n")
	runExpect(t, `p ("a"..."d").to_a`, `["a", "b", "c"]`+"\n")
	runExpect(t, `p ("a".."c").map { |c| c + "!" }`, `["a!", "b!", "c!"]`+"\n")
}

func TestEvalStringSucc(t *testing.T) {
	runExpect(t, `puts "a".succ`, "b\n")
	runExpect(t, `puts "z".succ`, "aa\n")
	runExpect(t, `puts "Az".succ`, "Ba\n")
	runExpect(t, `puts "99".succ`, "100\n")
}

func TestEvalRangeZipTakeDrop(t *testing.T) {
	runExpect(t, `p (1..5).zip([10, 20, 30])`, "[[1, 10], [2, 20], [3, 30], [4, nil], [5, nil]]\n")
	runExpect(t, `p (1..5).take(2)`, "[1, 2]\n")
	runExpect(t, `p (1..5).drop(2)`, "[3, 4, 5]\n")
}

func TestEvalDig(t *testing.T) {
	runExpect(t, `puts({a: {b: {c: 42}}}.dig(:a, :b, :c))`, "42\n")
	runExpect(t, `puts({a: 1}.dig(:x, :y).inspect)`, "nil\n")
	runExpect(t, `puts [1, [2, [3, 4]]].dig(1, 1, 0)`, "3\n")
}

func TestEvalArrayEachSliceCons(t *testing.T) {
	runExpect(t, `p [1, 2, 3, 4, 5].each_slice(2)`, "[[1, 2], [3, 4], [5]]\n")
	runExpect(t, `p [1, 2, 3, 4, 5].each_cons(2)`, "[[1, 2], [2, 3], [3, 4], [4, 5]]\n")
	src := `out = []
(1..5).each_cons(2) { |a, b| out << [a, b] }
p out`
	runExpect(t, src, "[[1, 2], [2, 3], [3, 4], [4, 5]]\n")
}

func TestEvalMethodMissing(t *testing.T) {
	src := `class Proxy
  def initialize(t); @t = t; end
  def method_missing(name, *args, &blk)
    @t.send(name, *args, &blk)
  end
end
p Proxy.new([1, 2, 3]).map { |x| x * 2 }
puts Proxy.new("hi").upcase`
	runExpect(t, src, "[2, 4, 6]\nHI\n")
}

func TestEvalSendForwardsBlock(t *testing.T) {
	runExpect(t, `p [1, 2, 3].send(:map) { |x| x * 10 }`, "[10, 20, 30]\n")
}

func TestEvalAmpNilNoBlock(t *testing.T) {
	src := `def f(&b); b.nil? ? "no" : "yes"; end
b = nil
puts f(&b)
puts f { 1 }`
	runExpect(t, src, "no\nyes\n")
}

func TestEvalArrayGroupByPartition(t *testing.T) {
	runExpect(t, `p [1, 2, 3, 4, 5].group_by { |n| n.even? }`, "{false=>[1, 3, 5], true=>[2, 4]}\n")
	runExpect(t, `p [1, 2, 3, 4, 5].partition { |n| n.even? }`, "[[2, 4], [1, 3, 5]]\n")
}

func TestEvalArrayMinMaxBy(t *testing.T) {
	runExpect(t, `puts [1, 2, 3, 4, 5].min_by { |n| (n - 3).abs }`, "3\n")
	runExpect(t, `puts ["a", "ab", "abcd", "abc"].max_by { |s| s.length }`, "abcd\n")
}

func TestEvalArrayTakeDropWhile(t *testing.T) {
	runExpect(t, `p [1, 2, 3, 4, 5].take_while { |n| n < 4 }`, "[1, 2, 3]\n")
	runExpect(t, `p [1, 2, 3, 4, 5].drop_while { |n| n < 3 }`, "[3, 4, 5]\n")
}

func TestEvalHashBlockMethods(t *testing.T) {
	runExpect(t, `p({a: 1, b: 2, c: 3}.find { |_, v| v == 2 })`, "[:b, 2]\n")
	runExpect(t, `puts({a: 1, b: 2}.any? { |_, v| v > 1 })`, "true\n")
	runExpect(t, `puts({a: 1, b: 2}.all? { |_, v| v > 0 })`, "true\n")
	runExpect(t, `puts({a: 1, b: 0}.count { |_, v| v > 0 })`, "1\n")
}

func TestEvalHashTransform(t *testing.T) {
	runExpect(t, `p({a: 1, b: 2}.transform_values { |v| v * 10 })`, "{:a=>10, :b=>20}\n")
	runExpect(t, `p({a: 1, b: 2}.transform_keys(&:to_s))`, `{"a"=>1, "b"=>2}`+"\n")
}

func TestEvalArrayOperators(t *testing.T) {
	runExpect(t, `p [1, 2] * 3`, "[1, 2, 1, 2, 1, 2]\n")
	runExpect(t, `p [1, 2, 3] + [4, 5]`, "[1, 2, 3, 4, 5]\n")
	runExpect(t, `p [1, 2, 3, 4] - [2, 4]`, "[1, 3]\n")
	runExpect(t, `p [1, 2, 3] & [2, 3, 4]`, "[2, 3]\n")
	runExpect(t, `p [1, 2, 3] | [3, 4, 5]`, "[1, 2, 3, 4, 5]\n")
	runExpect(t, `puts [1, 2, 3] * ","`, "1,2,3\n")
}

func TestEvalHashPlus(t *testing.T) {
	runExpect(t, `p({a: 1, b: 2} + {b: 20, c: 3})`, "{:a=>1, :b=>20, :c=>3}\n")
}

func TestEvalArrayFlattenDepth(t *testing.T) {
	runExpect(t, `p [[1, [2, [3]]]].flatten`, "[1, 2, 3]\n")
	runExpect(t, `p [[1, [2, [3]]]].flatten(1)`, "[1, [2, [3]]]\n")
	runExpect(t, `p [[1, [2, [3]]]].flatten(2)`, "[1, 2, [3]]\n")
}

func TestEvalHashClassIndex(t *testing.T) {
	runExpect(t, `p Hash[:a, 1, :b, 2]`, "{:a=>1, :b=>2}\n")
	runExpect(t, `p Hash[[[:a, 1], [:b, 2]]]`, "{:a=>1, :b=>2}\n")
	runExpect(t, `p Array[1, 2, 3]`, "[1, 2, 3]\n")
}

func TestEvalIntegerToSBase(t *testing.T) {
	runExpect(t, `puts 10.to_s(2)`, "1010\n")
	runExpect(t, `puts 255.to_s(16)`, "ff\n")
	runExpect(t, `puts 12.to_s(36)`, "c\n")
}

func TestEvalDefined(t *testing.T) {
	src := `class A; def m; 1; end; end
a = A.new
puts defined?(a)
puts defined?(b).inspect
puts defined?(a.m)
puts defined?(A)
puts defined?(NotDefined).inspect`
	runExpect(t, src, "local-variable\nnil\nmethod\nconstant\nnil\n")
}

func TestEvalStructNew(t *testing.T) {
	src := `Point = Struct.new(:x, :y)
p = Point.new(3, 4)
puts p.x
puts p.y
p.x = 10
puts p.x
p p.to_a
p p.members
puts p.class`
	runExpect(t, src, "3\n4\n10\n[10, 4]\n[:x, :y]\nPoint\n")
}

func TestEvalRecursiveTopLevelMethodWithBlock(t *testing.T) {
	// The recursive call passes &visit; the bare-name lookup must
	// see the top-level method (not the implicit main-Instance) so
	// the recursion finds itself, and the captured Proc must round-
	// trip on each recursion.
	src := `def walk(n, depth = 0, &visit)
  visit.call(n, depth)
  if n > 0
    walk(n - 1, depth + 1, &visit)
  end
end
out = []
walk(3) { |n, d| out << [n, d] }
p out`
	runExpect(t, src, "[[3, 0], [2, 1], [1, 2], [0, 3]]\n")
}

func TestEvalEventBus(t *testing.T) {
	src := `class Bus
  def initialize; @subs = Hash.new { |h, k| h[k] = [] }; end
  def on(ev, &h); @subs[ev] << h; self; end
  def emit(ev, *a); @subs[ev].each { |h| h.call(*a) }; end
end
b = Bus.new
b.on(:hi) { |n| puts "hi #{n}" }
b.on(:hi) { |n| puts "yo #{n}" }
b.emit(:hi, "world")`
	runExpect(t, src, "hi world\nyo world\n")
}

func TestEvalIntegerStep(t *testing.T) {
	src := `out = []
1.step(10, 2) { |i| out << i }
p out`
	runExpect(t, src, "[1, 3, 5, 7, 9]\n")
}

func TestEvalMathModule(t *testing.T) {
	runExpect(t, `puts Math.sqrt(16)`, "4.0\n")
	runExpect(t, `puts Math.hypot(3, 4)`, "5.0\n")
}

func TestEvalStringPad(t *testing.T) {
	runExpect(t, `puts "abc".center(9, "-")`, "---abc---\n")
	runExpect(t, `puts "abc".ljust(7, ".")`, "abc....\n")
	runExpect(t, `puts "abc".rjust(7, ".")`, "....abc\n")
}

func TestEvalBareKernelIdent(t *testing.T) {
	// Bare `puts` (no parens, no args) should fall back to Kernel.
	runExpect(t, `puts`, "\n")
}

func TestEvalBareVisibilityKeywordIsNoOp(t *testing.T) {
	// Bare `private` / `public` inside a class body must parse and
	// evaluate cleanly even though we don't track visibility yet.
	src := `class C
  def a; "a"; end
  private
  def b; "b"; end
end
puts C.new.a
puts C.new.b`
	runExpect(t, src, "a\nb\n")
}

func TestEvalKernelInteger(t *testing.T) {
	runExpect(t, `puts Integer("42")`, "42\n")
	runExpect(t, `puts Integer("0xff")`, "255\n")
	runExpect(t, `puts Integer("ff", 16)`, "255\n")
	runExpect(t, `puts Integer(3.7)`, "3\n")
}

func TestEvalKernelFloatStringArray(t *testing.T) {
	runExpect(t, `puts Float("1.5")`, "1.5\n")
	runExpect(t, `p String(42)`, `"42"`+"\n")
	runExpect(t, `p Array(nil)`, "[]\n")
	runExpect(t, `p Array(5)`, "[5]\n")
	runExpect(t, `p Array([1, 2])`, "[1, 2]\n")
}

func TestEvalKernelIntegerRaisesOnBadString(t *testing.T) {
	src := `begin
  Integer("hi")
rescue ArgumentError => e
  puts e.message
end`
	runExpect(t, src, `invalid value for Integer(): "hi"`+"\n")
}

func TestEvalUnaryMinusOnInstance(t *testing.T) {
	src := `class V
  attr_reader :n
  def initialize(n); @n = n; end
  def -@; V.new(-@n); end
  def to_s; "V#{@n}"; end
end
puts -V.new(7)`
	runExpect(t, src, "V-7\n")
}

func TestEvalArrayGrep(t *testing.T) {
	runExpect(t, `p [1, "a", :s, 2.0, "b"].grep(String)`, `["a", "b"]`+"\n")
	runExpect(t, `p (1..10).to_a.grep(3..6)`, "[3, 4, 5, 6]\n")
}

func TestEvalInstanceIndexGetSet(t *testing.T) {
	src := `class Bag
  def initialize; @h = {}; end
  def [](k); @h[k]; end
  def []=(k, v); @h[k] = v; end
end
b = Bag.new
b[:a] = 1
b[:b] = 2
puts b[:a]
puts b[:b]
puts b[:missing].inspect`
	runExpect(t, src, "1\n2\nnil\n")
}

func TestEvalStringIndex(t *testing.T) {
	runExpect(t, `puts "abc"[0]`, "a\n")
	runExpect(t, `puts "abc"[-1]`, "c\n")
	runExpect(t, `puts "hello"[1, 3]`, "ell\n")
	runExpect(t, `puts "hello"[1..3]`, "ell\n")
	runExpect(t, `puts "hello"[1...3]`, "el\n")
}

func TestEvalObjectSend(t *testing.T) {
	src := `class C; def greet(n); "hi #{n}"; end; end
puts C.new.send(:greet, "x")
puts C.new.send("greet", "y")`
	runExpect(t, src, "hi x\nhi y\n")
}

func TestEvalObjectMethod(t *testing.T) {
	src := `class C; def shout; "HI"; end; end
m = C.new.method(:shout)
puts m.call`
	runExpect(t, src, "HI\n")
}

func TestEvalComparableModule(t *testing.T) {
	src := `class P
  include Comparable
  def initialize(n); @n = n; end
  def <=>(o); @n <=> o.n; end
  def n; @n; end
end
puts P.new(2) < P.new(3)
puts P.new(5) >= P.new(5)`
	runExpect(t, src, "true\ntrue\n")
}

func TestEvalEachWithIndexNoBlock(t *testing.T) {
	runExpect(t, `p ["a", "b", "c"].each_with_index`, `[["a", 0], ["b", 1], ["c", 2]]`+"\n")
}

func TestEvalEachWithObject(t *testing.T) {
	src := `result = [1, 2, 3].each_with_object([]) { |x, acc| acc << x * 10 }
p result`
	runExpect(t, src, "[10, 20, 30]\n")
}

func TestEvalNumericCrossEqual(t *testing.T) {
	runExpect(t, `puts 3 == 3.0`, "true\n")
	runExpect(t, `puts 3.0 == 3`, "true\n")
	runExpect(t, `puts 3 == 4.0`, "false\n")
}

func TestEvalHeredoc(t *testing.T) {
	src := "text = <<~END\n  hello\n  world\nEND\n\nputs text"
	runExpect(t, src, "hello\nworld\n")
}

func TestEvalYieldMultiValue(t *testing.T) {
	src := `def pairs
  yield 1, "a"
  yield 2, "b"
end
pairs { |n, s| puts "#{n}=#{s}" }`
	runExpect(t, src, "1=a\n2=b\n")
}

func TestEvalRangeAggregates(t *testing.T) {
	runExpect(t, `puts (1..5).min`, "1\n")
	runExpect(t, `puts (1..5).max`, "5\n")
	runExpect(t, `puts (1...5).max`, "4\n")
	runExpect(t, `puts (1..5).sum`, "15\n")
}

func TestEvalRangeBlockMethods(t *testing.T) {
	runExpect(t, `p (1..10).select { |x| x.even? }`, "[2, 4, 6, 8, 10]\n")
	runExpect(t, `puts (1..10).any? { |x| x > 7 }`, "true\n")
	runExpect(t, `puts (1..5).reduce(0) { |s, x| s + x }`, "15\n")
}

func TestEvalReduceWithSymbol(t *testing.T) {
	runExpect(t, `puts (1..5).reduce(:+)`, "15\n")
	runExpect(t, `puts [1, 2, 3, 4].reduce(:*)`, "24\n")
	runExpect(t, `puts [1, 2, 3].reduce(10, :+)`, "16\n")
}

func TestEvalImplicitSelfCall(t *testing.T) {
	src := `class C
  def a; 1; end
  def b; a + 2; end
end
puts C.new.b`
	runExpect(t, src, "3\n")
}

func TestEvalImplicitSelfEnumerable(t *testing.T) {
	src := `class Bag
  include Enumerable
  def initialize(*xs); @xs = xs; end
  def each(&blk); @xs.each(&blk); end
  def small; select { |x| x < 5 }; end
end
p Bag.new(1, 2, 7, 3, 9).small`
	runExpect(t, src, "[1, 2, 3]\n")
}

func TestEvalSymbolCaptureForwardedThroughGoBlock(t *testing.T) {
	// `reject(&:done)` inside a method body must forward the
	// synthesised Proc through the implicit-self dispatch, and the
	// inner `each(&blk)` must wrap the go callback as a Proc so
	// `&blk` is non-nil.
	src := `class Item
  attr_accessor :ok
  def initialize(ok); @ok = ok; end
end
class L
  include Enumerable
  def initialize; @xs = []; end
  def add(x); @xs << x; self; end
  def each(&b); @xs.each(&b); end
  def pending; reject(&:ok); end
end
l = L.new.add(Item.new(true)).add(Item.new(false)).add(Item.new(true))
puts l.pending.count`
	runExpect(t, src, "1\n")
}

func TestEvalArrayCountNoBlock(t *testing.T) {
	runExpect(t, `puts [1, 2, 3].count`, "3\n")
	runExpect(t, `puts [1, 2, 2, 3].count(2)`, "2\n")
}

func TestEvalStringEachCharLine(t *testing.T) {
	src := `out = ""
"abc".each_char { |c| out << c.upcase }
puts out`
	runExpect(t, src, "ABC\n")
	runExpect(t, `"a\nb\nc".each_line { |l| puts l.chomp }`, "a\nb\nc\n")
}

func TestEvalArrayZipTakeDrop(t *testing.T) {
	runExpect(t, `p [1, 2, 3].zip([10, 20, 30])`, "[[1, 10], [2, 20], [3, 30]]\n")
	runExpect(t, `p [1, 2, 3].zip([10, 20], [100])`, "[[1, 10, 100], [2, 20, nil], [3, nil, nil]]\n")
	runExpect(t, `p [1, 2, 3, 4, 5].take(3)`, "[1, 2, 3]\n")
	runExpect(t, `p [1, 2, 3, 4, 5].drop(3)`, "[4, 5]\n")
}

func TestEvalArraySortMinMaxWithCustomSpaceship(t *testing.T) {
	src := `class P
  attr_reader :v
  def initialize(v); @v = v; end
  def <=>(o); @v <=> o.v; end
  def to_s; "P#{@v}"; end
end
xs = [P.new(3), P.new(1), P.new(2)]
puts xs.min
puts xs.max
puts xs.sort.map(&:to_s).join(",")`
	runExpect(t, src, "P1\nP3\nP1,P2,P3\n")
}

func TestEvalArrayDeepEqual(t *testing.T) {
	runExpect(t, `puts [1, [2, 3]] == [1, [2, 3]]`, "true\n")
	runExpect(t, `puts [1, 2] == [1, 2, 3]`, "false\n")
}

func TestEvalHashDeepEqual(t *testing.T) {
	runExpect(t, `puts({ a: 1, b: 2 } == { b: 2, a: 1 })`, "true\n")
	runExpect(t, `puts({ a: 1 } == { a: 2 })`, "false\n")
}

func TestEvalUserClassNewWithBlock(t *testing.T) {
	src := `class C
  def initialize(&blk)
    @blk = blk
  end
  def go(x); @blk.call(x); end
end
c = C.new { |x| x * 100 }
puts c.go(3)`
	runExpect(t, src, "300\n")
}

func TestEvalArrayAsHashKey(t *testing.T) {
	// Hash lookup uses rubyEqual on keys; Array deep-equality is what
	// makes the Memoizer idiom work.
	src := `h = {}
h[[1, 2]] = "a"
h[[3, 4]] = "b"
puts h[[1, 2]]
puts h[[3, 4]]
puts h[[1, 2]] == "a"`
	runExpect(t, src, "a\nb\ntrue\n")
}

func TestEvalHashNewWithDefaultBlock(t *testing.T) {
	src := `h = Hash.new { |hh, k| hh[k] = [] }
h[:a] << 1
h[:a] << 2
h[:b] << 9
p h`
	runExpect(t, src, "{:a=>[1, 2], :b=>[9]}\n")
}

func TestEvalHashNewWithDefaultValue(t *testing.T) {
	src := `h = Hash.new(0)
h[:x] += 1
h[:x] += 1
h[:y] += 1
p h
puts h[:never]`
	runExpect(t, src, "{:x=>2, :y=>1}\n0\n")
}

func TestEvalStringCapitalizeAndSplit(t *testing.T) {
	runExpect(t, `puts "hello".capitalize`, "Hello\n")
	runExpect(t, `puts "HELLO".swapcase`, "hello\n")
	// No-arg split treats runs of whitespace as one separator.
	runExpect(t, `p "  hello   world  ".split`, `["hello", "world"]`+"\n")
}

func TestEvalStringFormatOperator(t *testing.T) {
	runExpect(t, `puts "abc%dxyz" % [42]`, "abc42xyz\n")
	runExpect(t, `puts "pi=%.3f" % [3.14159]`, "pi=3.142\n")
	runExpect(t, `puts "x=%05d" % [7]`, "x=00007\n")
}

func TestEvalKernelSprintf(t *testing.T) {
	runExpect(t, `puts sprintf("%s=%d", "n", 5)`, "n=5\n")
	runExpect(t, `puts format("%.2f", 1.5)`, "1.50\n")
}

func TestEvalCustomExceptionWithSuper(t *testing.T) {
	src := `class AppError < StandardError
  def initialize(code, msg = nil)
    super(msg || "code: #{code}")
    @code = code
  end
  attr_reader :code
end
begin
  raise AppError.new(404, "not found")
rescue AppError => e
  puts "#{e.code}: #{e.message}"
end`
	runExpect(t, src, "404: not found\n")
}

func TestEvalBlockGiven(t *testing.T) {
	src := `def f
  if block_given?
    yield
  else
    "no block"
  end
end
puts f
puts f { "with block" }`
	runExpect(t, src, "no block\nwith block\n")
}

func TestEvalArraySumFloats(t *testing.T) {
	runExpect(t, `puts [1, 2.5, 3].sum`, "6.5\n")
	runExpect(t, `puts [1, 2, 3].sum`, "6\n")
}

func TestEvalLambdaClosure(t *testing.T) {
	src := `def counter(start)
  n = start
  ->(by = 1) { n += by; n }
end
c = counter(10)
puts c.call
puts c.call(5)
puts c.call`
	runExpect(t, src, "11\n16\n17\n")
}

func TestEvalCaseClassMatching(t *testing.T) {
	runExpect(t, `case 5
when Integer then puts "int"
when String then puts "str"
end`, "int\n")
	runExpect(t, `case "hi"
when Integer, Float then puts "num"
when String then puts "str"
end`, "str\n")
}

func TestEvalCaseRangeMatching(t *testing.T) {
	src := `case 5
when 1..3 then puts "low"
when 4..6 then puts "mid"
else puts "high"
end`
	runExpect(t, src, "mid\n")
}

func TestEvalSymbolMethods(t *testing.T) {
	runExpect(t, `puts :hi.to_s`, "hi\n")
	runExpect(t, `puts :hi.length`, "2\n")
	runExpect(t, `p :hi.upcase`, ":HI\n")
}

func TestEvalBoolNilStringMethods(t *testing.T) {
	runExpect(t, `puts true.to_s`, "true\n")
	runExpect(t, `puts false.to_s`, "false\n")
	runExpect(t, `p nil.to_s`, `""`+"\n")
	runExpect(t, `p nil.to_a`, "[]\n")
}

func TestEvalIntegerClampBetween(t *testing.T) {
	runExpect(t, `puts 5.clamp(1, 10)`, "5\n")
	runExpect(t, `puts 0.clamp(1, 10)`, "1\n")
	runExpect(t, `puts 15.clamp(1, 10)`, "10\n")
	runExpect(t, `puts 5.between?(1, 10)`, "true\n")
	runExpect(t, `puts 15.between?(1, 10)`, "false\n")
}

func TestEvalIntegerDivmodAndGcd(t *testing.T) {
	runExpect(t, `p 17.divmod(5)`, "[3, 2]\n")
	runExpect(t, `p (-17).divmod(5)`, "[-4, 3]\n")
	runExpect(t, `puts 12.gcd(18)`, "6\n")
}

func TestEvalInstanceDefaultEqual(t *testing.T) {
	src := `class A; end
a = A.new
b = A.new
puts a == a
puts a == b`
	runExpect(t, src, "true\nfalse\n")
}

func TestEvalEqualIdentity(t *testing.T) {
	src := `s1 = "abc"
s2 = "abc"
puts s1 == s2
puts s1.equal?(s2)
puts s1.equal?(s1)`
	runExpect(t, src, "true\nfalse\ntrue\n")
}

func TestEvalEnumerableDerivedMethods(t *testing.T) {
	src := `class Box
  include Enumerable
  def initialize(*xs); @xs = xs; end
  def each
    @xs.each { |x| yield x }
  end
end
b = Box.new(3, 1, 4, 1, 5)
p b.to_a
p b.map { |x| x * 10 }
p b.select { |x| x > 2 }
puts b.count
puts b.include?(4)
puts b.min
puts b.max
puts b.reduce(0) { |s, x| s + x }`
	runExpect(t, src, "[3, 1, 4, 1, 5]\n[30, 10, 40, 10, 50]\n[3, 4, 5]\n5\ntrue\n1\n5\n14\n")
}

func TestEvalStringAppendOp(t *testing.T) {
	src := `s = "foo"
s << "bar"
puts s`
	runExpect(t, src, "foobar\n")
}

func TestEvalArrayFirstN(t *testing.T) {
	runExpect(t, `p [1, 2, 3, 4, 5].first(3)`, "[1, 2, 3]\n")
	runExpect(t, `p [1, 2, 3, 4, 5].last(2)`, "[4, 5]\n")
}

func TestEvalArraySortBy(t *testing.T) {
	runExpect(t, `p [3, 1, 4, 1, 5].sort_by { |x| -x }`, "[5, 4, 3, 1, 1]\n")
}

func TestEvalArrayFindAnyAllNone(t *testing.T) {
	runExpect(t, `puts [1, 2, 3].find { |x| x > 1 }`, "2\n")
	runExpect(t, `puts [1, 2, 3].any? { |x| x > 2 }`, "true\n")
	runExpect(t, `puts [1, 2, 3].all? { |x| x > 0 }`, "true\n")
	runExpect(t, `puts [1, 2, 3].none? { |x| x > 5 }`, "true\n")
}

func TestEvalAutoSplatBlockArg(t *testing.T) {
	// `[[1, "a"], [2, "b"]].each { |i, s| ... }` -- the block has two
	// params but each yields one Array; the runtime destructures.
	src := `[[1, "a"], [2, "b"]].each { |i, s| puts "#{i}-#{s}" }`
	runExpect(t, src, "1-a\n2-b\n")
}

func TestEvalStringSubGsub(t *testing.T) {
	runExpect(t, `puts "hello".sub("l", "L")`, "heLlo\n")
	runExpect(t, `puts "hello".gsub("l", "L")`, "heLLo\n")
	runExpect(t, `puts "abc".sub("z", "Z")`, "abc\n")
}

func TestEvalIntegerChrAndStringOrd(t *testing.T) {
	runExpect(t, `puts 65.chr`, "A\n")
	runExpect(t, `puts "A".ord`, "65\n")
}

func TestEvalStringTr(t *testing.T) {
	runExpect(t, `puts "abc".tr("ab", "AB")`, "ABc\n")
}

func TestEvalKernelLoop(t *testing.T) {
	src := `i = 0
loop do
  i = i + 1
  break if i == 5
end
puts i`
	runExpect(t, src, "5\n")
}

func TestEvalHashSelectReject(t *testing.T) {
	runExpect(t, `p({ a: 1, b: 2, c: 3 }.select { |_, v| v > 1 })`, "{:b=>2, :c=>3}\n")
	runExpect(t, `p({ a: 1, b: 2, c: 3 }.reject { |_, v| v > 1 })`, "{:a=>1}\n")
}

func TestEvalMultiAssignWithSplat(t *testing.T) {
	src := `a, *b = [1, 2, 3, 4]
p a
p b`
	runExpect(t, src, "1\n[2, 3, 4]\n")
}

func TestEvalMultiAssignSplatMiddle(t *testing.T) {
	src := `a, *b, c = [1, 2, 3, 4, 5]
p a; p b; p c`
	runExpect(t, src, "1\n[2, 3, 4]\n5\n")
}

func TestEvalArrayMutation(t *testing.T) {
	src := `a = [1, 2]
a.push(3, 4)
p a
puts a.pop
p a
puts a.shift
a.unshift(0)
p a`
	runExpect(t, src, "[1, 2, 3, 4]\n4\n[1, 2, 3]\n1\n[0, 2, 3]\n")
}

func TestEvalArrayDelete(t *testing.T) {
	src := `a = [1, 2, 3, 2, 1]
a.delete(2)
p a`
	runExpect(t, src, "[1, 3, 1]\n")
}

func TestEvalHashMerge(t *testing.T) {
	src := `h = { a: 1, b: 2 }
p h.merge({ b: 20, c: 3 })`
	runExpect(t, src, "{:a=>1, :b=>20, :c=>3}\n")
}

func TestEvalHashDelete(t *testing.T) {
	src := `h = { a: 1, b: 2 }
puts h.delete(:a)
p h`
	runExpect(t, src, "1\n{:b=>2}\n")
}

func TestEvalHashFetch(t *testing.T) {
	runExpect(t, `puts({ a: 1 }.fetch(:a))`, "1\n")
	runExpect(t, `puts({ a: 1 }.fetch(:b, "default"))`, "default\n")
}

func TestEvalSplatInCall(t *testing.T) {
	src := `def f(a, b, c); [a, b, c]; end
args = [1, 2, 3]
p f(*args)`
	runExpect(t, src, "[1, 2, 3]\n")
}

func TestEvalSplatInArrayLiteral(t *testing.T) {
	src := `b = [2, 3, 4]
p [1, *b, 5]`
	runExpect(t, src, "[1, 2, 3, 4, 5]\n")
}

func TestEvalDoubleSplatKwargs(t *testing.T) {
	src := `def g(name:, age:); "#{name}/#{age}"; end
kw = { name: "z", age: 5 }
puts g(**kw)`
	runExpect(t, src, "z/5\n")
}

func TestEvalBreakStopsEach(t *testing.T) {
	src := `[1, 2, 3, 4].each do |x|
  break if x == 3
  puts x
end`
	runExpect(t, src, "1\n2\n")
}

func TestEvalBreakValueFromMap(t *testing.T) {
	// `break v` from inside .map exits and returns v.
	src := `r = [1, 2, 3, 4].map { |x| break "stopped" if x == 3; x * 2 }
puts r`
	runExpect(t, src, "stopped\n")
}

func TestEvalNextSkipsIteration(t *testing.T) {
	src := `r = [1, 2, 3, 4].map { |x| next 0 if x == 3; x }
p r`
	runExpect(t, src, "[1, 2, 0, 4]\n")
}

func TestEvalBreakInWhile(t *testing.T) {
	src := `i = 0
while true
  break if i == 3
  i = i + 1
end
puts i`
	runExpect(t, src, "3\n")
}

func TestEvalBreakInForIn(t *testing.T) {
	src := `for x in [10, 20, 30, 40]
  break if x == 30
  puts x
end`
	runExpect(t, src, "10\n20\n")
}

func TestEvalKernelPrint(t *testing.T) {
	runExpect(t, `print "a"; print "b"; print "\n"`, "ab\n")
}

func TestEvalIntegerAbsAndPredicates(t *testing.T) {
	runExpect(t, `puts (-5).abs`, "5\n")
	runExpect(t, `puts 5.succ`, "6\n")
	runExpect(t, `puts 5.pred`, "4\n")
	runExpect(t, `puts (-1).negative?`, "true\n")
	runExpect(t, `puts 0.positive?`, "false\n")
	runExpect(t, `puts 6.bit_length`, "3\n")
}

func TestEvalFloatMethods(t *testing.T) {
	runExpect(t, `puts 1.7.floor`, "1\n")
	runExpect(t, `puts 1.2.ceil`, "2\n")
	runExpect(t, `puts 1.5.round`, "2\n")
	runExpect(t, `puts (-1.5).round`, "-2\n")
	runExpect(t, `puts 3.7.to_i`, "3\n")
	runExpect(t, `puts (-2.5).abs`, "2.5\n")
}

func TestEvalArrayClassNew(t *testing.T) {
	runExpect(t, `p Array.new`, "[]\n")
	runExpect(t, `p Array.new(3)`, "[nil, nil, nil]\n")
	runExpect(t, `p Array.new(3, 0)`, "[0, 0, 0]\n")
}

func TestEvalIsAOnPrimitives(t *testing.T) {
	runExpect(t, `puts 5.is_a?(Integer)`, "true\n")
	runExpect(t, `puts "x".is_a?(String)`, "true\n")
	runExpect(t, `puts [].is_a?(Array)`, "true\n")
	runExpect(t, `puts({}.is_a?(Hash))`, "true\n")
	runExpect(t, `puts nil.is_a?(NilClass)`, "true\n")
}

func TestEvalOrAssignDefinesUndefined(t *testing.T) {
	// `x ||= 5` must assign when x is undefined, without raising
	// NameError on the read-side.
	src := `x ||= 5
puts x`
	runExpect(t, src, "5\n")
}

func TestEvalOrAssignKeepsTruthy(t *testing.T) {
	src := `x = 7
x ||= 99
puts x`
	runExpect(t, src, "7\n")
}

func TestEvalAndAssignTruthy(t *testing.T) {
	src := `y = 1
y &&= 10
puts y`
	runExpect(t, src, "10\n")
}

func TestEvalAndAssignFalsy(t *testing.T) {
	src := `y = nil
y &&= 10
puts y.inspect`
	runExpect(t, src, "nil\n")
}

func TestEvalCustomToS(t *testing.T) {
	src := `class C
  def initialize(n); @n = n; end
  def to_s; "C(#{@n})"; end
end
puts C.new(7)`
	runExpect(t, src, "C(7)\n")
}

func TestEvalComparableFromSpaceship(t *testing.T) {
	src := `class N
  def initialize(v); @v = v; end
  def <=>(other)
    if @v < other.value; -1
    elsif @v > other.value; 1
    else; 0
    end
  end
  def value; @v; end
end
a = N.new(3)
b = N.new(5)
puts a < b
puts a > b
puts a == N.new(3)
puts b.between?(N.new(1), N.new(10))`
	runExpect(t, src, "true\nfalse\ntrue\ntrue\n")
}

func TestEvalUserOperatorPlus(t *testing.T) {
	src := `class V
  attr_reader :n
  def initialize(n); @n = n; end
  def +(other); V.new(@n + other.n); end
  def to_s; "V(#{@n})"; end
end
puts V.new(2) + V.new(5)`
	runExpect(t, src, "V(7)\n")
}

func TestEvalSymbolToProc(t *testing.T) {
	runExpect(t, `p ["a", "b", "c"].map(&:upcase)`, `["A", "B", "C"]`+"\n")
}

func TestEvalAmpProcCapture(t *testing.T) {
	src := `f = ->(x) { x * 10 }
p [1, 2, 3].map(&f)`
	runExpect(t, src, "[10, 20, 30]\n")
}

func TestEvalIntegerUpto(t *testing.T) {
	src := `out = []
1.upto(4) { |i| out << i }
p out`
	runExpect(t, src, "[1, 2, 3, 4]\n")
}

func TestEvalIntegerDownto(t *testing.T) {
	src := `out = []
3.downto(1) { |i| out << i }
p out`
	runExpect(t, src, "[3, 2, 1]\n")
}

func TestEvalRespondTo(t *testing.T) {
	runExpect(t, `puts "hi".respond_to?(:upcase)`, "true\n")
	runExpect(t, `puts "hi".respond_to?(:wibble)`, "false\n")
}

func TestEvalFrozen(t *testing.T) {
	runExpect(t, `puts :sym.frozen?`, "true\n")
	runExpect(t, `puts 42.frozen?`, "true\n")
	runExpect(t, `puts "x".frozen?`, "false\n")
}

func TestEvalKeywordArgs(t *testing.T) {
	src := `def g(name:, age: 0)
  "#{name}/#{age}"
end
puts g(name: "x")
puts g(name: "y", age: 9)`
	runExpect(t, src, "x/0\ny/9\n")
}

// errorIs is a small helper to assert eval fails with a substring.
func evalForError(t *testing.T, src string) error {
	t.Helper()
	target := token.MustParseVersion("2.6")
	prog, err := parser.ParseFile("<test>", []byte(src), 0, parser.WithVersion(target))
	assert.NoError(t, err, "parse")
	var stdout bytes.Buffer
	env := object.NewMainEnvironment(object.WithVersion(target), object.WithStdout(&stdout))
	_, err = Eval(prog, env)
	return err
}

func TestEvalUnrescuedRaisePropagates(t *testing.T) {
	err := evalForError(t, `raise "kaboom"`)
	assert.NotNil(t, err)
	rs, ok := err.(*raiseSignal)
	assert.That(t, ok, "expected *raiseSignal, got %T", err)
	if ok {
		assert.Equal(t, "RuntimeError", rs.Exception.C.Name)
	}
}

func TestEvalMissingKeywordArgRaisesArgumentError(t *testing.T) {
	src := `def g(name:); name; end
g()`
	err := evalForError(t, src)
	assert.NotNil(t, err)
	assert.That(t, strings.Contains(err.Error(), "missing keyword"), "expected missing-keyword error, got %v", err)
}
