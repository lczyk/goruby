package parsetreenorm

import (
	"fmt"
	"strings"
)

// toFlat converts the indented parsetree dump form into a flat
// path-prefixed form, one fact per line. Path stability is the goal:
// inserting / removing a single node produces a contiguous insert /
// remove in the line diff, instead of a horizontal indent-cascade that
// would shift the entire surrounding subtree.
//
// Examples:
//
//	NODE_SCOPE
//	NODE_SCOPE.nd_tbl = (empty)
//	NODE_SCOPE.nd_args = nil
//	NODE_SCOPE.nd_body = NODE_FCALL
//	NODE_SCOPE.nd_body.nd_mid = :eval
//	NODE_SCOPE.nd_body.nd_args = NODE_ARRAY
//	NODE_SCOPE.nd_body.nd_args.nd_head[0] = NODE_OPCALL
//	NODE_SCOPE.nd_body.nd_args.nd_head[0].nd_mid = :+
//
// Repeated field names within a parent become indexed (`nd_head[0]`,
// `nd_head[1]`, ...). Singleton field names stay unindexed.
//
// Format of the input (relaxed -- we tolerate stray lines by skipping):
//   - `@ NODE_X` at indent D: opens a node. Its fields share indent D.
//   - `+- name: value` at indent D: leaf field of the most-recent node at D.
//   - `+- name:` at indent D: subtree field. Body sits at indent D+4.
//   - `(null node)` at indent D+4 inside a subtree field: null child.
//
// If parsing fails (no recognised tokens), the input is returned
// unchanged so partial / malformed dumps are still legible.
func toFlat(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	root, ok := parseTree(s)
	if !ok || root == nil {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	// Root: just the kind as a single line.
	b.WriteString(root.kind)
	b.WriteByte('\n')
	emitFlat(&b, root, root.kind)
	return b.String()
}

// --- parser -----------------------------------------------------------------

type ptField struct {
	name  string
	leaf  string  // non-empty for leaf field
	child *ptNode // non-nil for subtree field whose body is a node
	null  bool    // true when the body is `(null node)`
}

type ptNode struct {
	kind   string
	fields []ptField
}

// parseTree builds a node tree from the indented dump. Returns nil, false
// on totally-unrecognised input.
func parseTree(s string) (*ptNode, bool) {
	lines := strings.Split(s, "\n")
	// Trim leading blanks.
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) {
		return nil, false
	}
	depth, content := splitIndent(lines[i])
	kind, ok := parseNodeKind(content)
	if !ok {
		return nil, false
	}
	root := &ptNode{kind: kind}
	_, end := parseNodeFields(lines, i+1, depth, root)
	_ = end
	return root, true
}

// parseNodeFields consumes subsequent lines that are fields of n (all at
// the same baseDepth). Returns (advanced, end-index).
func parseNodeFields(lines []string, start, baseDepth int, n *ptNode) (int, int) {
	i := start
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		depth, content := splitIndent(line)
		if depth < baseDepth {
			return i, i
		}
		if depth > baseDepth {
			// Stray deeper line (no header before it). Skip.
			i++
			continue
		}
		// depth == baseDepth
		if rest, ok := strings.CutPrefix(content, "+- "); ok {
			name, val, hasVal := splitFieldNameValue(rest)
			f := ptField{name: name}
			if hasVal && val != "" {
				f.leaf = val
				n.fields = append(n.fields, f)
				i++
				continue
			}
			// Subtree field. Body lives at baseDepth+1 -- one indent level
			// deeper. Inspect that line.
			i++
			i = consumeFieldBody(lines, i, baseDepth+1, &f)
			n.fields = append(n.fields, f)
			continue
		}
		// `@ NODE_X` at baseDepth but we're inside a node already -- this
		// shouldn't occur in well-formed dumps but happens in the unwrapped
		// single-child NODE_BLOCK case where the unwrap leaves a child node
		// header at the same indent as its grandparent's other fields.
		// Treat the new node as a sibling (sentinel) -- caller decides.
		return i, i
	}
	return i, i
}

// consumeFieldBody reads the body (at bodyDepth) of a subtree field f.
// Body is either: `(null node)`, or `@ NODE_X` followed by that node's
// own fields. Returns the next unconsumed line index.
func consumeFieldBody(lines []string, start, bodyDepth int, f *ptField) int {
	i := start
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) {
		return i
	}
	depth, content := splitIndent(lines[i])
	if depth != bodyDepth {
		// Body missing / misaligned. Leave field as-is.
		return i
	}
	switch {
	case content == "(null node)":
		f.null = true
		return i + 1
	case strings.HasPrefix(content, "@ "):
		kind, ok := parseNodeKind(content)
		if !ok {
			return i + 1
		}
		child := &ptNode{kind: kind}
		f.child = child
		next, _ := parseNodeFields(lines, i+1, bodyDepth, child)
		return next
	}
	// Unrecognised body shape -- skip the line, leave field empty.
	return i + 1
}

// parseNodeKind validates `@ NODE_X...` prefix and extracts NODE_X. Only
// names matching `NODE_[A-Z0-9_]+` are accepted -- a stricter rule than
// the raw dump suggests, but matches every observed MRI kind and rejects
// fuzz-synthetic junk like `@ #0000` that would otherwise produce
// non-idempotent flat output.
func parseNodeKind(content string) (string, bool) {
	rest, ok := strings.CutPrefix(content, "@ ")
	if !ok {
		return "", false
	}
	if !strings.HasPrefix(rest, "NODE_") {
		return "", false
	}
	end := len(rest)
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			end = i
			break
		}
	}
	return rest[:end], end > len("NODE_")
}

// splitIndent counts the indent of `line` (in 4-col units) and returns
// (depth, content). Indent chars are spaces and `|`. Each 4-char chunk
// counts as one depth level.
func splitIndent(line string) (int, string) {
	depth := 0
	i := 0
	for i+3 < len(line) {
		c0 := line[i]
		// each level: "    " or "|   "
		if (c0 == ' ' || c0 == '|') &&
			line[i+1] == ' ' && line[i+2] == ' ' && line[i+3] == ' ' {
			depth++
			i += 4
			continue
		}
		break
	}
	return depth, line[i:]
}

// splitFieldNameValue parses `name: value` or `name:` into (name, value,
// hasValue). `name` may contain `->`. Splits on the first `: ` after the
// name, or trailing `:` with nothing after.
func splitFieldNameValue(s string) (string, string, bool) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return s, "", false
	}
	name := s[:idx]
	rest := s[idx+1:]
	if rest == "" {
		return name, "", true
	}
	// `name:` with empty body is a subtree field; leading space + content
	// is a leaf value.
	if rest[0] == ' ' {
		return name, rest[1:], true
	}
	// `name:<non-space>...` is rare; treat as leaf preserving the value.
	return name, rest, true
}

// --- emitter ----------------------------------------------------------------

// emitFlat walks n.fields and writes one path-prefixed fact per line to
// b. nodePath is the full path to n itself (does NOT end with field names).
// Subtree fields emit `<nodePath>.<field> = NODE_X` then recurse.
func emitFlat(b *strings.Builder, n *ptNode, nodePath string) {
	// Count siblings per field name to decide [N] indexing.
	counts := map[string]int{}
	for _, f := range n.fields {
		counts[f.name]++
	}
	seen := map[string]int{}
	for _, f := range n.fields {
		fieldPath := nodePath + "." + f.name
		if counts[f.name] > 1 {
			idx := seen[f.name]
			seen[f.name] = idx + 1
			fieldPath = fmt.Sprintf("%s.%s[%d]", nodePath, f.name, idx)
		}
		switch {
		case f.null:
			b.WriteString(fieldPath)
			b.WriteString(" = nil\n")
		case f.child != nil:
			b.WriteString(fieldPath)
			b.WriteString(" = ")
			b.WriteString(f.child.kind)
			b.WriteByte('\n')
			emitFlat(b, f.child, fieldPath)
		default:
			b.WriteString(fieldPath)
			b.WriteString(" = ")
			b.WriteString(f.leaf)
			b.WriteByte('\n')
		}
	}
}
