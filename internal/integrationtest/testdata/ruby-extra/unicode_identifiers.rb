# Unicode identifier adversarial tests.
# NOTE: this file contains non-ASCII characters intentionally.
# It tests arabic, devanagari, thai, cyrillic, CJK, greek, and accented latin
# identifiers in various Ruby contexts.

# ---- basic unicode identifiers ----
عربي = 1
हिन्दी = 2
ภาษา = 3
переменная = 4
メソッド = 5
変数 = 6
μέθοδος = 7
déf = 8

# ---- unicode method names with ? and ! ----
def صحيح?
  true
end

def اختبر!
  :ok
end

def メソッド?
  true
end

# ---- unicode in class/instance/global variables ----
@العربية = 1
@@हिन्दी = 2
$переменная = 3
@μέθοδος = 4

# ---- unicode identifiers in expressions ----
αβ = 1
γδ = 2
result = αβ + γδ

# ---- unicode identifiers as method calls ----
puts عربي
puts हिन्दी

# ---- unicode in string interpolation ----
"hello #{переменная}"
"#{αβ} + #{γδ} = #{result}"

# ---- unicode constant-like (treated as IDENT, not CONST) ----
Établissement = "Paris"
Ærø = "Denmark"

# NOTE: the line below ensures the file ends with a newline
