package token

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestPosIsValid(t *testing.T) {
	assert.That(t, !NoPos.IsValid(), "NoPos.IsValid() should be false")
	assert.That(t, Pos(1).IsValid(), "Pos(1).IsValid() should be true")
}

func TestPositionString(t *testing.T) {
	cases := []struct {
		name string
		p    Position
		want string
	}{
		{"invalid no file", Position{}, "-"},
		{"invalid with file", Position{Filename: "a.rb"}, "a.rb"},
		{"file line col", Position{Filename: "a.rb", Line: 12, Column: 3}, "a.rb:12:3"},
		{"line col only", Position{Line: 1, Column: 10}, "1:10"},
		{"line only", Position{Filename: "a.rb", Line: 5}, "a.rb:5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.p.String(), c.want)
		})
	}
}

func TestFilePosition(t *testing.T) {
	src := []byte("abc\ndef\n\nghi")
	f := NewFile("x.rb", len(src))
	for i, b := range src {
		if b == '\n' {
			f.AddLine(i + 1)
		}
	}
	cases := []struct {
		off       int
		line, col int
	}{
		{0, 1, 1},  // 'a'
		{2, 1, 3},  // 'c'
		{3, 1, 4},  // '\n' on line 1
		{4, 2, 1},  // 'd'
		{8, 3, 1},  // empty line
		{9, 4, 1},  // 'g'
		{11, 4, 3}, // 'i'
	}
	for _, c := range cases {
		p := f.Pos(c.off)
		got := f.Position(p)
		assert.Equal(t, got.Line, c.line)
		assert.Equal(t, got.Column, c.col)
		assert.Equal(t, got.Filename, "x.rb")
		assert.Equal(t, got.Offset, c.off)
	}
}

func TestFilePositionInvalid(t *testing.T) {
	f := NewFile("x.rb", 10)
	assert.That(t, !f.Position(NoPos).IsValid(), "NoPos should be invalid")
	assert.That(t, !f.Position(Pos(1000)).IsValid(), "out-of-range Pos should be invalid")
}

func TestFileAddLineOutOfOrder(t *testing.T) {
	f := NewFile("x.rb", 100)
	f.AddLine(10)
	f.AddLine(5)  // ignored
	f.AddLine(10) // duplicate, ignored
	f.AddLine(20)
	assert.Equal(t, f.LineCount(), 3) // implicit 0, then 10, then 20
}
