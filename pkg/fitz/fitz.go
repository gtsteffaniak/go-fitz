// Package fitz reads PDF documents in pure Go. It wraps the internal PDF port
// and has no cgo and no runtime library dependency.
//
// Only the methods the port implements are provided. Rendering, text, HTML and
// SVG output, and the other document formats are not ported yet.
package fitz

import (
	"errors"

	pdfport "github.com/gtsteffaniak/go-fitz/internal/mupdf/pdf"
)

// Errors.
var (
	// ErrEmptyBytes is returned when a document is opened from an empty buffer.
	ErrEmptyBytes = errors.New("fitz: cannot send empty bytes")
	// ErrNotPDF is returned when the data is not a PDF document.
	ErrNotPDF = pdfport.ErrNotPDF
	// ErrCorrupt is returned when the document structure cannot be read.
	ErrCorrupt = pdfport.ErrCorrupt
	// ErrUnsupported is returned by methods whose feature is not ported yet.
	ErrUnsupported = pdfport.ErrUnsupported
	// ErrPageMissing is returned when a page number is out of range.
	ErrPageMissing = pdfport.ErrPageMissing
)

// Outline is one entry of the table of contents.
type Outline struct {
	// Level is the hierarchy level of the entry, starting from 1.
	Level int
	// Title of the outline item.
	Title string
	// URI is the destination displayed when this outline item is activated.
	URI string
	// Page is the page number of an internal link.
	Page int
	// Top is the vertical position on the destination page.
	Top float64
}

// Link is an external link on a page.
type Link struct {
	URI string
}
