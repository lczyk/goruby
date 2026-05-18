package token

import (
	"sort"
	"strconv"
)

// Pos encodes a source location as offset+1, so the zero value (NoPos) is
// invalid. Use File.Offset(p) / File.Pos(offset) to convert.
type Pos int

// NoPos signals an unknown / unset source position.
const NoPos Pos = 0

// Pos converts a byte offset into the file to a Pos.
func (f *File) Pos(offset int) Pos { return Pos(offset + 1) }

// Offset returns the byte offset encoded by p. Returns -1 if p is invalid.
func (f *File) Offset(p Pos) int {
	if !p.IsValid() {
		return -1
	}
	return int(p) - 1
}

// IsValid reports whether p is a real position (not NoPos).
func (p Pos) IsValid() bool { return p != NoPos }

// Position is a fully resolved source location.
type Position struct {
	Filename string
	Offset   int
	Line     int
	Column   int
}

// IsValid reports whether the position has a known line.
func (pos Position) IsValid() bool { return pos.Line > 0 }

// String formats the position as "file:line:col", "line:col" when Filename
// is empty, or "-" when invalid. Matches go/token.Position.String.
func (pos Position) String() string {
	s := pos.Filename
	if pos.IsValid() {
		if s != "" {
			s += ":"
		}
		s += strconv.Itoa(pos.Line)
		if pos.Column != 0 {
			s += ":" + strconv.Itoa(pos.Column)
		}
	}
	if s == "" {
		s = "-"
	}
	return s
}

// File holds the line table for one source file. One File per parse; there
// is no FileSet.
type File struct {
	Filename string
	size     int
	lines    []int32 // byte offset of the start of each line; lines[0] is always 0
}

// NewFile returns a File for src. The line table starts with the implicit
// first line at offset 0; lexer feeds further newlines via AddLine.
func NewFile(filename string, size int) *File {
	return &File{
		Filename: filename,
		size:     size,
		lines:    []int32{0},
	}
}

// Name returns the file's name.
func (f *File) Name() string { return f.Filename }

// Size returns the source size in bytes.
func (f *File) Size() int { return f.size }

// LineCount returns the number of lines recorded in the table.
func (f *File) LineCount() int { return len(f.lines) }

// AddLine records that a new line starts at byte offset off. Calls must
// arrive in strictly increasing offset order; out-of-order or duplicate
// offsets are ignored.
func (f *File) AddLine(off int) {
	if off < 0 || off > f.size {
		return
	}
	if n := len(f.lines); n > 0 && int(f.lines[n-1]) >= off {
		return
	}
	f.lines = append(f.lines, int32(off))
}

// Position resolves p into a Position. Returns the zero Position if p is
// NoPos or outside the file.
func (f *File) Position(p Pos) Position {
	if !p.IsValid() {
		return Position{Filename: f.Filename}
	}
	off := int(p) - 1
	if off < 0 || off > f.size {
		return Position{Filename: f.Filename}
	}
	i := max(sort.Search(len(f.lines), func(i int) bool {
		return int(f.lines[i]) > off
	})-1, 0)
	return Position{
		Filename: f.Filename,
		Offset:   off,
		Line:     i + 1,
		Column:   off - int(f.lines[i]) + 1,
	}
}
