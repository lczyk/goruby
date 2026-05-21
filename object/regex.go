package object

import "regexp"

// Regex wraps a compiled Go regexp.Regexp for use as a ruby Regexp
// value. Source / options are kept for inspect output.
type Regex struct {
	RE      *regexp.Regexp
	Source  string
	Options string
}

func NewRegex(re *regexp.Regexp, source, options string) *Regex {
	return &Regex{RE: re, Source: source, Options: options}
}

func (r *Regex) Type() Type       { return OBJECT_OBJ }
func (r *Regex) Class() RubyClass { return RegexpClass }
func (r *Regex) Inspect() string {
	return "/" + r.Source + "/" + r.Options
}
