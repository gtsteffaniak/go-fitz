// Package pdf is a pure Go port of the PDF object, cross-reference and page
// layers of MuPDF 1.28.0 (source/pdf/pdf-parse.c, pdf-xref.c, pdf-page.c,
// pdf-stream.c and friends). It is ported in file-for-file order where
// practical so upgrades can be diffed against upstream.
//
// Upstream: https://github.com/ArtifexSoftware/mupdf (tag 1.28.0, see
// scripts/fetch-mupdf-source.sh). MuPDF is AGPL-3.0; this package is too.
//
// This milestone covers the structure layer only: object parsing, xref
// tables and streams, object streams, Flate with PNG predictors, the page
// tree, the Info dictionary and page bounds. Anything beyond that returns
// ErrUnsupported so the compare gate reports it as typed_unsupported rather
// than silently wrong.
package pdf

import "fmt"

// Object is any PDF object: Null, Bool, Integer, Real, Name, String, Array,
// Dict, Stream or Ref.
type Object interface {
	isObject()
}

// Null is the PDF null object.
type Null struct{}

// Bool is a PDF boolean.
type Bool bool

// Integer is a PDF integer.
type Integer int64

// Real is a PDF real number.
type Real float64

// Name is a PDF name without the leading slash and with #xx escapes decoded.
type Name string

// String is a PDF string (literal or hexadecimal), raw bytes.
type String []byte

// Array is a PDF array.
type Array []Object

// Dict is a PDF dictionary keyed by name.
type Dict map[Name]Object

// Stream is a PDF stream: its dictionary and the undecoded bytes.
type Stream struct {
	Dict Dict
	Raw  []byte
}

// Ref is an indirect reference "num gen R".
type Ref struct {
	Num int
	Gen int
}

func (Null) isObject()    {}
func (Bool) isObject()    {}
func (Integer) isObject() {}
func (Real) isObject()    {}
func (Name) isObject()    {}
func (String) isObject()  {}
func (Array) isObject()   {}
func (Dict) isObject()    {}
func (*Stream) isObject() {}
func (Ref) isObject()     {}

// Errors returned by this package. Callers map these to the public fitz
// sentinel errors; ErrUnsupported marks a document or feature this port does
// not implement yet.
var (
	// ErrNotPDF is returned when the input has no PDF header.
	ErrNotPDF = fmt.Errorf("pdf: not a PDF document")
	// ErrCorrupt is returned when the structure cannot be read. MuPDF would
	// attempt repair; repair is not ported yet, so this is also reported as
	// unsupported by the public fitz package.
	ErrCorrupt = fmt.Errorf("pdf: corrupt document structure")
	// ErrUnsupported marks a feature that this port does not implement yet.
	ErrUnsupported = fmt.Errorf("pdf: unsupported feature")
)

// unsupported wraps ErrUnsupported with the feature name.
func unsupported(feature string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, feature)
}

// corrupt wraps ErrCorrupt with detail.
func corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrCorrupt}, args...)...)
}
