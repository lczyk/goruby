package ast

type keyValue struct {
	Key   Expression
	Value Expression
}

// OrderedExprMap is an insertion-ordered map from Expression to Expression.
type OrderedExprMap struct {
	entries []keyValue
}

func NewOrderedExprMap() *OrderedExprMap {
	return &OrderedExprMap{}
}

func (m *OrderedExprMap) Set(key, value Expression) {
	for i := range m.entries {
		if m.entries[i].Key == key {
			m.entries[i].Value = value
			return
		}
	}
	m.entries = append(m.entries, keyValue{key, value})
}

func (m *OrderedExprMap) Get(key Expression) (Expression, bool) {
	for _, kv := range m.entries {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return nil, false
}

func (m *OrderedExprMap) Len() int {
	return len(m.entries)
}

func (m *OrderedExprMap) Entries() []keyValue {
	return m.entries
}
