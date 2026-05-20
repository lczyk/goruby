#!/usr/bin/env ruby
# Generate .in fixture programs for each esolang interpreter under
# internal/integrationtest/testdata/esolang_tests/<lang>/. After running
# this script, follow up with `scripts/esolang-tests-oracle` to write
# the matching .expected files via MRI.
#
# Strategy:
# - Canonical brainfuck "Hello, World!" is translated for every esolang
#   that's a brainfuck dialect (full or partial).
# - Esolangs with limited opcode sets (tick: no decrement, no loops;
#   pluso: only increment-mod-27) get bespoke hello world programs.
# - rot13_ruby: hello world / fizzbuzz / fibonacci as rot13'd ruby.
# - simplified_emmental: hello world via stack push + putc decimals.
# - arsel: hello world via the lowercase letter board.
# - kittykittymewmew, ellipsis, skinny_pig: skipped (no feasible program
#   shape -- the interpreter never runs user code, or produces a fixed
#   non-text output, or has a complex modal token grammar).

require "fileutils"

ROOT = File.expand_path("../internal/integrationtest/testdata/esolang_tests", __dir__)

# Canonical brainfuck "Hello, World!\n" (Wikipedia variant).
BF_HELLO = '++++++++[>++++[>++>+++>+++>+<<<<-]>+>+>->>+[<]<-]>>.>---.+++++++..+++.>>.<-.<.+++.------.--------.>>+.>++.'

# Mapping table per esolang for the eight brainfuck ops.
# Order matters because some destination strings overlap when ops are
# adjacent: write the source through one gsub call with the full hash to
# avoid double-substitution.
BF_MAPS = {
  "mariofuck"  => {">"=>"w", "<"=>"a", "+"=>"s", "-"=>"d", "."=>"p", ","=>"q", "["=>"m", "]"=>"n"},
  "raylang"    => {">"=>"r", "<"=>"R", "+"=>"a", "-"=>"A", "."=>"y", ","=>"Y", "["=>"l", "]"=>"L"},
  "fuckbees"   => {">"=>"u", "<"=>"f", "+"=>"c", "-"=>"k", "."=>"b", ","=>"e", "["=>"E", "]"=>"s"},
  "cupid"      => {">"=>">>", "<"=>"<<", "+"=>">-", "-"=>"-<", "."=>"->", ","=>"<-", "["=>"--", "]"=>"<>"},
  # Token-based: emit space-separated words. The token regex inside each
  # interpreter happily eats the spaces (`/./` matches and maps to '').
  "fuckscript" => {">"=>"right ", "<"=>"left ", "+"=>"up ", "-"=>"down ", "."=>"out ", ","=>"in ", "["=>"fuck ", "]"=>"shit "},
  "screamcode" => {">"=>"AAAH ", "<"=>"AAAAGH ", "+"=>"FUCK ", "-"=>"SHIT ", "."=>"!!!!!! ", ","=>"WHAT?! ", "["=>"OW ", "]"=>"OWIE "},
  "babylang"   => {">"=>"gaga ", "<"=>"gugu ", "+"=>"aaag ", "-"=>"uuug ", "."=>"guuu ", ","=>"gaaa ", "["=>"gagu ", "]"=>"guga "},
  # nope tokens self-terminate with `!`; no spaces needed.
  "nope"       => {">"=>"...!", "<"=>"..!", "+"=>"!", "-"=>".!", "."=>".....!", ","=>"....!", "["=>"......!", "]"=>".......!"},
  # spoon: binary-coded brainfuck. Variable-length tokens; the interp's
  # longest-first regex tokenises them unambiguously.
  "spoon"      => {">"=>"010", "<"=>"011", "+"=>"1", "-"=>"000", "."=>"001010", ","=>"0010110", "["=>"00100", "]"=>"0011"},
  # la_wea: space-separated word tokens. `weón` is the +1 op; we keep
  # it utf-8 so the source matches the interp's hash keys verbatim.
  "la_wea"     => {">"=>"puta ", "<"=>"chucha ", "+"=>"weón ", "-"=>"maricón ", "."=>"ctm ", ","=>"quéweá ", "["=>"pichula ", "]"=>"tula "},
}

def translate_bf(map, src)
  src.gsub(/[><+\-.,\[\]]/, map)
end

def write_fixture(lang, name, content)
  dir = File.join(ROOT, lang)
  FileUtils.mkdir_p(dir)
  path = File.join(dir, "#{name}.in")
  # Strip trailing whitespace so the pre-commit hook is happy. Token-
  # based dialects (fuckscript, screamcode, babylang) end their final
  # token with a space separator that's purely cosmetic.
  content = content.sub(/[ \t]+\z/, "")
  File.write(path, content)
  puts "wrote #{path} (#{content.bytesize}B)"
end

# Brainfuck variants: hello world via canonical translation.
BF_MAPS.each do |lang, map|
  write_fixture(lang, "hello_world", translate_bf(map, BF_HELLO))
end

# tick: > < + * only. No decrement. No loops. Build hello world by
# walking forward and accumulating each cell from 0 up to ascii.
def tick_hello
  out = +""
  "Hello, World!\n".each_char.with_index do |c, i|
    out << ">" if i > 0
    out << ("+" * c.ord) << "*"
  end
  out
end
write_fixture("tick", "hello_world", tick_hello)

# pluso: c starts at 1, p increments c mod 27, o outputs c+64 (or space
# at c==0). Only upper-case A-Z + space. "HELLO WORLD" via mod-27 walk.
def pluso_hello
  letters = "HELLO WORLD".chars.map { |ch| ch == " " ? 0 : ch.ord - 64 }
  out = +""
  c = 1
  letters.each do |target|
    delta = (target - c) % 27
    out << ("p" * delta) << "o"
    c = target
  end
  out
end
write_fixture("pluso", "hello_world", pluso_hello)

# raylang already has its hello via BF_MAPS; the earlier A.in fixture
# remains.
# mariofuck/fuckbees/etc likewise -- A.in stays as the minimal smoke
# test, hello_world.in is the new canonical one.

# simplified_emmental: digits accumulate decimal into c, '#' pushes c
# then resets it, '.' putc c then pops c off m. To print N chars in
# order, push the last N-1 chars in reverse onto m, load the first
# char into c, then emit N periods. After the last period c is nil but
# no further code runs.
def emmental_str(text)
  bytes = text.bytes
  push = bytes.reverse[0...-1].map { |b| "#{b}#" }.join
  push + bytes.first.to_s + ("." * bytes.length)
end
write_fixture("simplified_emmental", "hello_world", emmental_str("Hello, World!\n"))
# Existing single-letter fixtures (A.in, AB.in) get regenerated with
# the same push-pop pattern so they don't blow up on m.pop=nil.
write_fixture("simplified_emmental", "A", emmental_str("A"))
write_fixture("simplified_emmental", "AB", emmental_str("AB"))

# rot13_ruby: input is rot13'd ruby source.
def rot13(s)
  s.tr("A-Za-z", "N-ZA-Mn-za-m")
end
write_fixture("rot13_ruby", "hello_world", rot13('puts "hello world"') + "\n")
write_fixture("rot13_ruby", "fizzbuzz", rot13(<<~RUBY))
  (1..15).each do |i|
    if i % 15 == 0
      puts "FizzBuzz"
    elsif i % 3 == 0
      puts "Fizz"
    elsif i % 5 == 0
      puts "Buzz"
    else
      puts i
    end
  end
RUBY
write_fixture("rot13_ruby", "fibonacci", rot13(<<~RUBY))
  a = 0
  b = 1
  10.times do
    puts a
    a, b = b, a + b
  end
RUBY

# arsel: forward-walking pointer over a lowercase-board. `<` resets to
# the start. The board has a-z, 1-9, 0, space. Write "hello world".
def arsel_hello
  board = ("a".."z").to_a + ("1".."9").to_a + ["0"] + [" "]
  out = +""
  pos = 0
  "hello world".each_char do |ch|
    target = board.index(ch)
    raise "no slot for #{ch.inspect}" unless target
    if target >= pos
      out << ("+" * (target - pos)) << "0"
    else
      out << "<" << ("+" * target) << "0"
    end
    pos = target
  end
  out
end
write_fixture("arsel", "hello_world", arsel_hello)

# developers: first gsub maps "Developers" -> 'o' and any other char ->
# ' '. Second gsub maps o-counts -> commands. So a source of "ooo "
# becomes the 3-o command (p+=1). Walk the canonical bf and emit space-
# separated o-runs.
DEV_OPS = {">"=>3, "<"=>4, "+"=>1, "-"=>2, "."=>6, ","=>5, "["=>7, "]"=>8}
def developers_translate(src)
  src.chars.map { |c| "o" * DEV_OPS.fetch(c) }.join(" ")
end
write_fixture("developers", "hello_world", developers_translate(BF_HELLO))

# cupid hello already via BF_MAPS but also keep the existing A.in.

# jr: no loops; `]` increments m[p], `>` advances p, `,` putc. Walk
# forward and accumulate each cell to its ascii code before printing.
def jr_str(text)
  out = +""
  text.each_char.with_index do |c, i|
    out << ">" if i > 0
    out << ("]" * c.ord) << ","
  end
  out
end
write_fixture("jr", "hello_world", jr_str("Hello, World!\n"))

# kittykittymewmew: the interp only defines `run_code` and never calls
# it, so any input produces empty stdout. We commit an empty fixture
# to document the parity (goruby and MRI both produce nothing).
write_fixture("kittykittymewmew", "empty", "")

puts "done."
