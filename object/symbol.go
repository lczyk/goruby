package object

// Symbol represents a ruby Symbol value as an ID into the SymbolPool of
// the owning environment.
// Pointer-free; noscan-eligible.
//
// Symbol equality reduces to ID comparison; no string compare. Hashes
// keyed by symbols can store the ID directly, avoiding per-key allocation.
type Symbol struct {
	ID int32
}

func (s *Symbol) Type() Type       { return SYMBOL_OBJ }
func (s *Symbol) Class() RubyClass { return nil }

// Inspect on a bare Symbol cannot resolve its name without access to the
// owning pool. The evaluator routes formatting through Inspect(obj, v) in
// inspect.go, which has the pool in scope. This bare Inspect is a fallback
// that prints `:<id>` so accidental calls produce identifiable output.
func (s *Symbol) Inspect() string { return ":<sym:" + itoa32(s.ID) + ">" }

// SymbolPool interns symbol names, mirroring the parser's LitPool pattern:
// one slice grows for the pool's lifetime, individual Symbol values hold
// only an int32 offset. The pool itself is scanned by the GC, but only
// once -- per-symbol scan cost is zero.
type SymbolPool struct {
	byID   []string         // ID -> name; grow-only
	byName map[string]int32 // intern set
}

// NewSymbolPool returns an empty pool.
func NewSymbolPool() *SymbolPool {
	return &SymbolPool{
		byName: make(map[string]int32),
	}
}

// Intern returns the Symbol value for name, creating a fresh entry if
// needed.
func (p *SymbolPool) Intern(name string) *Symbol {
	if id, ok := p.byName[name]; ok {
		return &Symbol{ID: id}
	}
	id := int32(len(p.byID))
	p.byID = append(p.byID, name)
	p.byName[name] = id
	return &Symbol{ID: id}
}

// Name returns the source name for ID. Panics if the ID was never
// interned by this pool (programmer error: symbol from foreign pool).
func (p *SymbolPool) Name(id int32) string {
	if id < 0 || int(id) >= len(p.byID) {
		panic("object.SymbolPool: unknown symbol ID")
	}
	return p.byID[id]
}

// itoa32 is a tiny allocation-free int32 -> string for the fallback path.
func itoa32(n int32) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [11]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
