package token

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "latest"},
		{"3", "3.4"},
		{"2", "2.7"},
		{"4", "4.0"},
		{"1", "1.9"},
		{"3.0", "3.0"},
		{"2.7", "2.7"},
		{"1.9", "1.9"},
	}
	for _, tt := range tests {
		v, err := ParseVersion(tt.input)
		if err != nil {
			t.Errorf("ParseVersion(%q): %v", tt.input, err)
			continue
		}
		if got := v.String(); got != tt.want {
			t.Errorf("ParseVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseVersionErrors(t *testing.T) {
	for _, input := range []string{"abc", "3.1.2", "99"} {
		_, err := ParseVersion(input)
		if err == nil {
			t.Errorf("ParseVersion(%q): expected error", input)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		v, other string
		want     bool
	}{
		{"", "4.0", true},
		{"3.0", "2.7", true},
		{"3.0", "3.0", true},
		{"3.0", "3.1", false},
		{"2.7", "3.0", false},
		{"1.9", "1.9", true},
	}
	for _, tt := range tests {
		v := MustParseVersion(tt.v)
		other := MustParseVersion(tt.other)
		if got := v.AtLeast(other); got != tt.want {
			t.Errorf("%s.AtLeast(%s) = %v, want %v", tt.v, tt.other, got, tt.want)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"3.0", "3.0", 0},
		{"3.1", "3.0", 1},
		{"3.0", "3.1", -1},
		{"4.0", "3.4", 1},
		{"", "", 0},
		{"", "4.0", 1},
		{"4.0", "", -1},
	}
	for _, tt := range tests {
		a := MustParseVersion(tt.a)
		b := MustParseVersion(tt.b)
		if got := a.Compare(b); got != tt.want {
			t.Errorf("%s.Compare(%s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
