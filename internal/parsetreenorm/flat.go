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
	// Fast-path: already-flat input has no `@ ` header anywhere. Skip
	// parseTree entirely (saves 1 alloc on the idempotent path).
	if !strings.Contains(s, "@ ") {
		return s
	}
	root, ok := parseTree(s)
	if !ok || root == nil {
		return s
	}
	normalizeTree(root)
	var b strings.Builder
	b.Grow(len(s))
	// Root: just the kind as a single line.
	b.WriteString(root.kind)
	b.WriteByte('\n')
	emitFlat(&b, root, root.kind)
	return b.String()
}

// normalizeTree recursively collapses no-op wrappers made trivial by
// earlier passes or introduced by prism but absent from MRI 2.x dumps:
//
//  1. NODE_BLOCK collapse (see blockTrivialContent):
//     a. stripNullBeginChildren removes nd_head children pointing to
//        NODE_BEGIN(null). On MRI 2.x single-line def bodies
//        (`def f(); X; end`), this leaves a NODE_BLOCK with no nd_head,
//        only an nd_next pointing into the rest of the stmt-chain --
//        collapse the dangling BLOCK.
//     b. After (a), a NODE_BLOCK with a single nd_head and nil nd_next
//        (or no nd_next) is a 1-element list -- replace with the head.
//  2. ParenthesesNode around a single-statement StatementsNode (see
//     parenthesesTrivialContent): `(1)` and `1` evaluate identically, but
//     prism preserves the ParenthesesNode wrapper while MRI 2.x collapses
//     it. Strip the wrapper so cross-version comparisons match.
//  3. BeginNode with no rescue / else / ensure clauses and a single-stmt
//     statements child (see beginTrivialContent): `begin; X; end` and `X`
//     evaluate identically; prism keeps the BeginNode where MRI 2.x
//     collapses.
//
// The inner loop cascades replacements -- collapsing BeginNode can yield
// a node whose own collapse rule applies (e.g. an inner ParenthesesNode).
//
// After 1:1 collapse, a multi-stmt no-clause BeginNode left in a
// StatementsNode body slot is spliced into the parent's body list (one
// child becomes N) -- standalone `begin; a; b; end` and bare `a; b` then
// produce the same flat tree. Splice is gated on parent being a
// StatementsNode so we never splice into expression-value slots, where
// the BeginNode wrapper has real last-value semantics.
func normalizeTree(n *ptNode) {
	if n == nil {
		return
	}
	for i := range n.fields {
		f := &n.fields[i]
		if f.child == nil {
			continue
		}
		normalizeTree(f.child)
		for {
			replacement, ok := trivialWrapperContent(f.child)
			if !ok {
				break
			}
			if replacement == nil {
				f.child = nil
				f.null = true
				break
			}
			f.child = replacement
		}
	}
	if n.kind == "StatementsNode" {
		spliceMultiStmtBegin(n)
	}
}

// spliceMultiStmtBegin expands any no-clause multi-stmt BeginNode in n's
// body fields into the parent's body list inline. Single-stmt BeginNodes
// have already been collapsed by trivialWrapperContent before this runs,
// so the BeginNodes seen here have multi-stmt statements bodies.
func spliceMultiStmtBegin(n *ptNode) {
	// Pre-scan: skip allocation entirely when there's nothing to splice.
	expand := false
	for _, f := range n.fields {
		if f.name == "body" && f.child != nil && f.child.kind == "BeginNode" {
			if _, ok := beginMultiStmtBodies(f.child); ok {
				expand = true
				break
			}
		}
	}
	if !expand {
		return
	}
	out := make([]ptField, 0, len(n.fields))
	for _, f := range n.fields {
		if f.name == "body" && f.child != nil && f.child.kind == "BeginNode" {
			if bodies, ok := beginMultiStmtBodies(f.child); ok {
				for _, b := range bodies {
					out = append(out, ptField{name: "body", child: b})
				}
				continue
			}
		}
		out = append(out, f)
	}
	n.fields = out
}

// beginMultiStmtBodies returns the inner StatementsNode body children of
// a no-clause BeginNode with multi-stmt statements (len > 1). Returns
// (nil, false) for single-stmt (handled by beginTrivialContent), empty
// statements, or any clause set.
func beginMultiStmtBodies(n *ptNode) ([]*ptNode, bool) {
	if n.kind != "BeginNode" {
		return nil, false
	}
	var stmts *ptNode
	for _, f := range n.fields {
		switch f.name {
		case "statements":
			stmts = f.child
		case "rescue_clause", "else_clause", "ensure_clause":
			if !(f.null || f.leaf == "nil") {
				return nil, false
			}
		case "BeginNodeFlags":
			// Cosmetic flag, ignore.
		default:
			return nil, false
		}
	}
	if stmts == nil || stmts.kind != "StatementsNode" {
		return nil, false
	}
	var bodies []*ptNode
	for _, f := range stmts.fields {
		if f.name != "body" {
			return nil, false
		}
		if f.child == nil {
			return nil, false
		}
		bodies = append(bodies, f.child)
	}
	if len(bodies) < 2 {
		return nil, false
	}
	return bodies, true
}

// trivialWrapperContent returns the inner content of n iff n is one of
// the recognised no-op wrapper shapes. See normalizeTree for the list.
func trivialWrapperContent(n *ptNode) (*ptNode, bool) {
	if n == nil {
		return nil, false
	}
	if r, ok := blockTrivialContent(n); ok {
		return r, true
	}
	if r, ok := parenthesesTrivialContent(n); ok {
		return r, true
	}
	if r, ok := beginTrivialContent(n); ok {
		return r, true
	}
	return nil, false
}

// parenthesesTrivialContent collapses `ParenthesesNode -> StatementsNode
// -> single body` to the bare body. Multi-stmt parens (`(a; b; c)`) keep
// last-value semantics that the bare body wouldn't -- leave alone.
// Empty parens (`()` -> nil) and parens whose body isn't a StatementsNode
// are also left alone (the latter shouldn't occur in valid prism dumps).
func parenthesesTrivialContent(n *ptNode) (*ptNode, bool) {
	if n.kind != "ParenthesesNode" {
		return nil, false
	}
	var body *ptNode
	for _, f := range n.fields {
		switch f.name {
		case "body":
			if f.child == nil {
				return nil, false
			}
			if body != nil {
				return nil, false
			}
			body = f.child
		case "ParenthesesNodeFlags":
			// Cosmetic flag field (prism 4.0). Ignore.
		default:
			// Unknown field -- be conservative.
			return nil, false
		}
	}
	if body == nil || body.kind != "StatementsNode" {
		return nil, false
	}
	return statementsSingleChild(body)
}

// beginTrivialContent collapses `BeginNode { statements: stmts, rescue:
// nil, else: nil, ensure: nil }` to the single child of stmts when stmts
// is a single-stmt StatementsNode. Multi-stmt body would need splicing
// into the parent's statement list, which the current list-field model
// doesn't support -- leave alone.
func beginTrivialContent(n *ptNode) (*ptNode, bool) {
	if n.kind != "BeginNode" {
		return nil, false
	}
	var stmts *ptNode
	for _, f := range n.fields {
		switch f.name {
		case "statements":
			stmts = f.child
		case "rescue_clause", "else_clause", "ensure_clause":
			if !(f.null || f.leaf == "nil") {
				return nil, false
			}
		case "BeginNodeFlags":
			// Cosmetic flag, ignore.
		default:
			return nil, false
		}
	}
	if stmts == nil || stmts.kind != "StatementsNode" {
		return nil, false
	}
	return statementsSingleChild(stmts)
}

// statementsSingleChild returns the single body child of a StatementsNode
// or (nil, false) if it has zero or more-than-one body fields.
func statementsSingleChild(stmts *ptNode) (*ptNode, bool) {
	var single *ptNode
	for _, f := range stmts.fields {
		if f.name != "body" {
			return nil, false
		}
		if single != nil {
			return nil, false
		}
		single = f.child
	}
	if single == nil {
		return nil, false
	}
	return single, true
}

// blockTrivialContent returns (replacement, true) if n is a NODE_BLOCK
// that should be collapsed: either it has only an nd_next chain (head
// was stripped) or a single nd_head with no surviving nd_next.
// Returns (nil, true) for a fully-empty BLOCK (caller should set null).
func blockTrivialContent(n *ptNode) (*ptNode, bool) {
	if n.kind != "NODE_BLOCK" {
		return nil, false
	}
	var head *ptNode
	var next *ptNode
	var nextNull bool
	for _, f := range n.fields {
		switch f.name {
		case "nd_head":
			if head != nil { // already have one head -- this is a multi-head BLOCK
				return nil, false
			}
			head = f.child
		case "nd_next":
			next = f.child
			nextNull = f.null
		}
	}
	switch {
	case head == nil && next == nil && !nextNull:
		// no children at all -- empty
		return nil, true
	case head == nil && nextNull:
		// `BLOCK[<stripped>, nil]` -> empty
		return nil, true
	case head == nil && next != nil:
		// `BLOCK[<stripped>, REST]` -> REST
		return next, true
	case head != nil && (next == nil || nextNull):
		// `BLOCK[X, nil]` -> X
		return head, true
	}
	return nil, false
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
		// Field markers: `+- ` (MRI) or `+-- ` (Prism). Try both.
		var rest string
		var ok bool
		if rest, ok = strings.CutPrefix(content, "+-- "); !ok {
			rest, ok = strings.CutPrefix(content, "+- ")
		}
		if ok {
			// Prism inlines child nodes as `+-- @ NodeName ...` -- detect
			// and create a subtree field with the child node directly.
			if afterAt, isInline := strings.CutPrefix(rest, "@ "); isInline {
				if kind, kok := parseNodeKind("@ " + afterAt); kok {
					child := &ptNode{kind: kind}
					i++
					i, _ = parseNodeFields(lines, i, baseDepth+1, child)
					// Anonymous field (kind is the body) -- use "_" so emitter
					// still produces a fact line.
					n.fields = append(n.fields, ptField{name: "_", child: child})
					continue
				}
			}
			name, val, hasVal := splitFieldNameValue(rest)
			// Prism list field: `+-- name: (length: N)` followed by N
			// inline `+-- @ Child` entries at bodyDepth. Expand to N
			// separate ptFields named `name`, each owning one child.
			isList := hasVal && strings.HasPrefix(val, "(length: ") &&
				strings.HasSuffix(val, ")")
			if isList {
				i++
				i = consumeListField(lines, i, baseDepth+1, name, n)
				continue
			}
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

// consumeListField reads N inline `+-- @ Child` entries at bodyDepth and
// appends one ptField named fieldName per child to n.fields. Used for
// Prism list fields (`+-- name: (length: N)`).
func consumeListField(lines []string, start, bodyDepth int, fieldName string, n *ptNode) int {
	i := start
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) == "" {
			i++
			continue
		}
		depth, content := splitIndent(lines[i])
		if depth < bodyDepth {
			return i
		}
		if depth > bodyDepth {
			// Deeper line without a header at bodyDepth -- shouldn't
			// happen, but skip defensively.
			i++
			continue
		}
		// Look for `+-- @ ChildNode` (inline child at this indent).
		after, hasMarker := strings.CutPrefix(content, "+-- ")
		if !hasMarker {
			return i // end of list (next sibling field of parent)
		}
		afterAt, isNode := strings.CutPrefix(after, "@ ")
		if !isNode {
			return i // non-node entry -- not part of this list
		}
		kind, kok := parseNodeKind("@ " + afterAt)
		if !kok {
			return i
		}
		child := &ptNode{kind: kind}
		i++
		i, _ = parseNodeFields(lines, i, bodyDepth+1, child)
		n.fields = append(n.fields, ptField{name: fieldName, child: child})
	}
	return i
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

// parseNodeKind validates `@ <Name>...` prefix and extracts Name. Accepts
// MRI dump form (`NODE_[A-Z0-9_]+`, e.g. `NODE_SCOPE`) and Prism dump form
// (CamelCase, e.g. `ProgramNode`, `DefNode`). Rejects fuzz-synthetic junk
// like `@ #0000` that would otherwise produce non-idempotent flat output.
func parseNodeKind(content string) (string, bool) {
	rest, ok := strings.CutPrefix(content, "@ ")
	if !ok {
		return "", false
	}
	if len(rest) == 0 {
		return "", false
	}
	// MRI form: NODE_<UPPER>
	if strings.HasPrefix(rest, "NODE_") {
		end := len(rest)
		for i := 0; i < len(rest); i++ {
			c := rest[i]
			if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
				end = i
				break
			}
		}
		if end <= len("NODE_") {
			return "", false
		}
		return rest[:end], true
	}
	// Prism form: <UpperCamelCase>[Node|Flags]
	first := rest[0]
	if !(first >= 'A' && first <= 'Z') {
		return "", false
	}
	end := len(rest)
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '_') {
			end = i
			break
		}
	}
	return rest[:end], end > 0
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
