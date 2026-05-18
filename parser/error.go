package parser

import (
	"bytes"
	"fmt"
	gotoken "go/token"

	"github.com/lczyk/goruby/token"
	"github.com/pkg/errors"
)

// Make sure unexpectedTokenError implements error interface
var _ error = &unexpectedTokenError{}

// Make sure Errors implements error interface
var _ error = &Errors{}

// Make sure parseError implements error interface
var _ error = &parseError{}

// ErrorKind classifies a parser-side error so callers can filter by category
// rather than by message text.
type ErrorKind int

const (
	// LexError originates from the lexer and was surfaced as an ILLEGAL token.
	LexError ErrorKind = iota
	// SyntaxError covers parser-side rejections (bad expression shape, malformed
	// literal, unexpected token shape not caught by unexpectedTokenError).
	SyntaxError
)

func (k ErrorKind) String() string {
	switch k {
	case LexError:
		return "lex error"
	case SyntaxError:
		return "syntax error"
	}
	return "error"
}

// parseError is a structured parser error carrying source position and kind.
type parseError struct {
	Pos  gotoken.Position
	Kind ErrorKind
	Msg  string
}

func (e *parseError) Error() string {
	if e.Pos.Filename != "" || e.Pos.IsValid() {
		return fmt.Sprintf("%s: %s: %s", e.Pos.String(), e.Kind, e.Msg)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Msg)
}

// IsLexError reports whether err contains a LexError parseError.
func IsLexError(err error) bool { return hasErrorKind(err, LexError) }

// IsSyntaxError reports whether err contains a SyntaxError parseError.
func IsSyntaxError(err error) bool { return hasErrorKind(err, SyntaxError) }

func hasErrorKind(err error, kind ErrorKind) bool {
	if errs, ok := err.(*Errors); ok {
		for _, e := range errs.errors {
			if hasErrorKind(e, kind) {
				return true
			}
		}
		return false
	}
	pe, ok := errors.Cause(err).(*parseError)
	if !ok {
		return false
	}
	return pe.Kind == kind
}

// NewErrors returns a composite Error object wrapping multiple errors into
// one.
func NewErrors(context string, errors ...error) *Errors {
	return &Errors{context, errors}
}

// Errors represents a group of errors and its context
//
// Errors implements the error interface to be used as an error in the code.
type Errors struct {
	context string
	errors  []error
}

// Error returns all error messages divided by newlines and prepended with the
// error context.
func (e *Errors) Error() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s:\n", e.context)
	for _, err := range e.errors {
		fmt.Fprintf(&buf, "\t%s\n", err.Error())
	}
	return buf.String()
}

// IsEOFError returns true if err represents an unexpectedTokenError with its
// actual token type set to token.EOF.
//
// It returns false for any other error.
func IsEOFError(err error) bool {
	if errors, ok := err.(*Errors); ok {
		for _, e := range errors.errors {
			if IsEOFError(e) {
				return true
			}
		}
	}

	cause := errors.Cause(err)
	tokenErr, ok := cause.(*unexpectedTokenError)
	if !ok {
		return false
	}
	if tokenErr.actualToken != token.EOF {
		return false
	}

	return true
}

// IsEOFInsteadOfNewlineError returns true if err represents an unexpectedTokenError with its
// actual token type set to token.EOF and if its expected token types includes
// token.NEWLINE.
//
// It returns false for any other error.
func IsEOFInsteadOfNewlineError(err error) bool {
	if !IsEOFError(err) {
		return false
	}

	if errors, ok := err.(*Errors); ok {
		for _, e := range errors.errors {
			if IsEOFInsteadOfNewlineError(e) {
				return true
			}
		}
	}

	tokenErr := errors.Cause(err).(*unexpectedTokenError)

	for _, expectedToken := range tokenErr.expectedTokens {
		if expectedToken == token.NEWLINE {
			return true
		}
	}

	return false
}

type tokens []token.Type

func (t tokens) String() string {
	if len(t) == 1 {
		return t[0].String()
	}
	var s []token.Type = t
	return fmt.Sprintf("%s", s)
}

type unexpectedTokenError struct {
	Pos            gotoken.Position
	expectedTokens []token.Type
	actualToken    token.Type
	actualLiteral  string // optional; included in Error() if non-empty
}

func (e *unexpectedTokenError) Error() string {
	actual := e.actualToken.String()
	if e.actualLiteral != "" && e.actualLiteral != actual {
		actual = fmt.Sprintf("%s (%q)", e.actualToken, e.actualLiteral)
	}
	msg := fmt.Sprintf(
		"unexpected %s, expecting %s",
		actual,
		tokens(e.expectedTokens),
	)
	if e.Pos.Filename != "" || e.Pos.IsValid() {
		return e.Pos.String() + ": " + msg
	}
	return msg
}
