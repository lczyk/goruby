package parsetreenorm

import (
	"strings"
	"testing"
)

func TestNormalizeIdempotent(t *testing.T) {
	cases := []string{
		"",
		"@ NODE_LIT\n+- nd_lit: 42\n",
		sampleDump3x,
	}
	for i, in := range cases {
		once := Normalize(in)
		twice := Normalize(once)
		if once != twice {
			t.Errorf("case %d: not idempotent\n--- once ---\n%s\n--- twice ---\n%s", i, once, twice)
		}
	}
}

func TestStripsHeader(t *testing.T) {
	in := "###########################################################\n" +
		"## Do NOT use this node dump for any purpose other than  ##\n" +
		"## debug and research.  Compatibility is not guaranteed. ##\n" +
		"###########################################################\n\n" +
		"# @ NODE_LIT\n# +- nd_lit: 42\n"
	out := Normalize(in)
	if strings.Contains(out, "Do NOT use") {
		t.Errorf("header banner not stripped:\n%s", out)
	}
	if strings.Contains(out, "# @") {
		t.Errorf("leading hash not stripped:\n%s", out)
	}
}

func TestStripsLineAndID(t *testing.T) {
	in := "# @ NODE_LIT (id: 5, line: 12, location: (1,0)-(1,2))\n# +- nd_lit: 42\n"
	out := Normalize(in)
	if strings.Contains(out, "line:") || strings.Contains(out, "id:") {
		t.Errorf("line/id not stripped:\n%s", out)
	}
}

func TestStripsSiblingIndex(t *testing.T) {
	in := "@ NODE_BLOCK\n+- nd_head (1):\n|   @ NODE_LIT\n+- nd_head (2):\n    @ NODE_LIT\n"
	out := Normalize(in)
	if strings.Contains(out, "(1)") || strings.Contains(out, "(2)") {
		t.Errorf("sibling index not stripped:\n%s", out)
	}
}

func TestStripsNullBegin(t *testing.T) {
	in := "@ NODE_BLOCK\n" +
		"+- nd_head:\n" +
		"|   @ NODE_BEGIN\n" +
		"|   +- nd_body:\n" +
		"|       (null node)\n" +
		"+- nd_head:\n" +
		"    @ NODE_LIT\n" +
		"    +- nd_lit: 42\n"
	out := Normalize(in)
	// After stripping NODE_BEGIN(null) the wrapping NODE_BLOCK has one
	// remaining child and gets unwrapped too.
	if strings.Contains(out, "NODE_BEGIN") {
		t.Errorf("NODE_BEGIN(null) not stripped:\n%s", out)
	}
	if strings.Contains(out, "NODE_BLOCK") {
		t.Errorf("single-child NODE_BLOCK not unwrapped:\n%s", out)
	}
	if !strings.Contains(out, "NODE_LIT") || !strings.Contains(out, "nd_lit: 42") {
		t.Errorf("body content lost:\n%s", out)
	}
}

func TestKeepsMultiChildBlock(t *testing.T) {
	in := "@ NODE_BLOCK\n" +
		"+- nd_head:\n" +
		"|   @ NODE_LIT\n" +
		"|   +- nd_lit: 1\n" +
		"+- nd_head:\n" +
		"    @ NODE_LIT\n" +
		"    +- nd_lit: 2\n"
	out := Normalize(in)
	if !strings.Contains(out, "NODE_BLOCK") {
		t.Errorf("multi-child NODE_BLOCK should NOT be unwrapped:\n%s", out)
	}
}

const sampleDump3x = `###########################################################
## Do NOT use this node dump for any purpose other than  ##
## debug and research.  Compatibility is not guaranteed. ##
###########################################################

# @ NODE_SCOPE (id: 24, line: 1, location: (1,0)-(4,3))
# +- nd_tbl: (empty)
# +- nd_args:
# |   (null node)
# +- nd_body:
#     @ NODE_DEFN (id: 1, line: 1, location: (1,0)-(4,3))*
#     +- nd_mid: :foo
#     +- nd_defn:
#         @ NODE_SCOPE (id: 23, line: 4, location: (1,0)-(4,3))
#         +- nd_tbl: :x
#         +- nd_args:
#         |   (null node)
#         +- nd_body:
#             @ NODE_LIT (id: 6, line: 2, location: (2,7)-(2,8))
#             +- nd_lit: 42
`
