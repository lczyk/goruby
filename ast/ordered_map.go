package ast

type keyValue struct {
	Key     Expression
	Value   Expression
	Omitted bool // hash value omission: {x:} means {x: x}
}

// OrderedExprMap is an insertion-ordered map from Expression to Expression.
type OrderedExprMap struct {
	entries []keyValue
}

func NewOrderedExprMap() *OrderedExprMap {
	return &OrderedExprMap{}
}

// Set appends a new entry. The parser builds AST nodes fresh per pair, so
// keys are never structurally re-set; the lookup-and-replace path the
// previous implementation did was dead weight (O(n^2) over hash-literal size).
// If a future caller genuinely needs dedup-by-pointer, do it at the call site.
func (m *OrderedExprMap) Set(key, value Expression) {
	m.entries = append(m.entries, keyValue{Key: key, Value: value})
}

func (m *OrderedExprMap) Get(key Expression) (Expression, bool) {
	for _, kv := range m.entries {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return nil, false
}

// SetOmitted marks the most recently inserted entry as a hash value omission
// (ruby 3.1+ {x:} syntax). All current parser callsites invoke this
// immediately after Set on the same key, so the last entry is always the
// right target.
func (m *OrderedExprMap) SetOmitted(key Expression) {
	if n := len(m.entries); n > 0 && m.entries[n-1].Key == key {
		m.entries[n-1].Omitted = true
	}
}

func (m *OrderedExprMap) Len() int {
	return len(m.entries)
}

func (m *OrderedExprMap) Entries() []keyValue {
	return m.entries
}

// Reset truncates the entries slice to zero length while preserving the
// backing array's capacity. Used by Arena.NewHashLiteral to reuse the
// slot's hashmap storage across parses -- the slice header stays alive
// over Reset so the next parse's Set() calls fill the same backing array.
//
// The retained slice still holds references to keyValue entries from the
// prior parse until they get overwritten. Callers that need to free those
// references sooner should call ResetClear instead.
func (m *OrderedExprMap) Reset() {
	// Zero each entry so the prior parse's Expression references become
	// collectible -- the slice cap is reused, so without zeroing the
	// arena slab would keep those AST sub-trees pinned via the keyValue
	// pointer fields.
	for i := range m.entries {
		m.entries[i] = keyValue{}
	}
	m.entries = m.entries[:0]
}
