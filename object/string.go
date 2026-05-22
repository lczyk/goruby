package object

// String represents a mutable ruby String. Holds a []byte buffer so
// in-place mutation (<<, gsub!, etc.) does not break go's string
// immutability.
//
// Not noscan-eligible: the Buf slice header carries a pointer to the
// backing array. Acceptable cost -- mutable strings are inherently
// pointer-bearing.
type String struct {
	Buf      []byte
	frozen   bool
	encoding string // MRI-style display name, e.g. "UTF-8"; "" = default UTF-8
}

// Encoding returns the string's encoding name (display form). Empty
// string means the default UTF-8.
func (s *String) Encoding() string {
	if s.encoding == "" {
		return "UTF-8"
	}
	return s.encoding
}

// SetEncoding tags the string with the given encoding name. Does not
// transcode the buffer (mirrors MRI's String#force_encoding).
func (s *String) SetEncoding(name string) { s.encoding = name }

// Frozen reports whether the String was frozen via Object#freeze.
// Mutation methods test this and raise FrozenError if true.
func (s *String) Frozen() bool { return s.frozen }

// Freeze marks the String frozen. Idempotent.
func (s *String) Freeze() { s.frozen = true }

// NewString returns a String wrapping a fresh copy of s. Copying ensures
// the resulting String can be mutated without aliasing the caller's
// memory.
func NewString(s string) *String {
	b := make([]byte, len(s))
	copy(b, s)
	return &String{Buf: b}
}

// NewStringFromBytes returns a String taking ownership of b. The caller
// must not retain b after the call.
func NewStringFromBytes(b []byte) *String { return &String{Buf: b} }

func (s *String) Value() string { return string(s.Buf) }
func (s *String) Inspect() string {
	// Minimal inspect: wrap value in double quotes with a few standard
	// escapes. Sufficient for the literals corpus; expand as needed.
	var out []byte
	out = append(out, '"')
	for _, c := range s.Buf {
		switch c {
		case '"', '\\':
			out = append(out, '\\', c)
		case '\n':
			out = append(out, '\\', 'n')
		case '\t':
			out = append(out, '\\', 't')
		case '\r':
			out = append(out, '\\', 'r')
		default:
			out = append(out, c)
		}
	}
	out = append(out, '"')
	return string(out)
}

func (s *String) Type() Type       { return STRING_OBJ }
func (s *String) Class() RubyClass { return StringClass }

// FrozenString is the interned, immutable string form. The ID resolves to
// the source text via a StringPool on the owning environment.
// Pointer-free; noscan-eligible.
type FrozenString struct {
	ID int32
}

func (f *FrozenString) Type() Type       { return STRING_OBJ }
func (f *FrozenString) Class() RubyClass { return StringClass }

// Inspect on a bare FrozenString cannot resolve its text without the pool;
// the evaluator routes through Inspect(obj, v) in inspect.go. This
// fallback emits an identifying marker.
func (f *FrozenString) Inspect() string { return "\"<frozen:" + itoa32(f.ID) + ">\"" }

// StringPool interns frozen string literals, mirroring SymbolPool.
type StringPool struct {
	byID   []string
	byName map[string]int32
}

// NewStringPool returns an empty pool.
func NewStringPool() *StringPool {
	return &StringPool{byName: make(map[string]int32)}
}

// Intern returns a FrozenString for s, creating a fresh entry if needed.
func (p *StringPool) Intern(s string) *FrozenString {
	if id, ok := p.byName[s]; ok {
		return &FrozenString{ID: id}
	}
	id := int32(len(p.byID))
	p.byID = append(p.byID, s)
	p.byName[s] = id
	return &FrozenString{ID: id}
}

// Get returns the source text for ID. Panics on unknown ID.
func (p *StringPool) Get(id int32) string {
	if id < 0 || int(id) >= len(p.byID) {
		panic("object.StringPool: unknown frozen-string ID")
	}
	return p.byID[id]
}
