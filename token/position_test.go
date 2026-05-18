package token

import "testing"

func TestPosIsValid(t *testing.T) {
	if NoPos.IsValid() {
		t.Errorf("NoPos.IsValid() = true, want false")
	}
	if !Pos(1).IsValid() {
		t.Errorf("Pos(1).IsValid() = false, want true")
	}
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
			if got := c.p.String(); got != c.want {
				t.Errorf("String() = %q, want %q", got, c.want)
			}
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
		if got.Line != c.line || got.Column != c.col {
			t.Errorf("Position(off=%d) = %d:%d, want %d:%d", c.off, got.Line, got.Column, c.line, c.col)
		}
		if got.Filename != "x.rb" {
			t.Errorf("Position(off=%d).Filename = %q, want %q", c.off, got.Filename, "x.rb")
		}
		if got.Offset != c.off {
			t.Errorf("Position(off=%d).Offset = %d, want %d", c.off, got.Offset, c.off)
		}
	}
}

func TestFilePositionInvalid(t *testing.T) {
	f := NewFile("x.rb", 10)
	if p := f.Position(NoPos); p.IsValid() {
		t.Errorf("Position(NoPos) IsValid, want invalid; got %+v", p)
	}
	if p := f.Position(Pos(1000)); p.IsValid() {
		t.Errorf("Position(out-of-range) IsValid, want invalid; got %+v", p)
	}
}

func TestFileAddLineOutOfOrder(t *testing.T) {
	f := NewFile("x.rb", 100)
	f.AddLine(10)
	f.AddLine(5) // ignored
	f.AddLine(10) // duplicate, ignored
	f.AddLine(20)
	if f.LineCount() != 3 { // implicit 0, then 10, then 20
		t.Errorf("LineCount() = %d, want 3", f.LineCount())
	}
}
