package fitz

import (
	"image"
	"io"
	"os"

	pdfport "github.com/gtsteffaniak/go-fitz/internal/mupdf/pdf"
)

// Document is an opened PDF. It is safe for concurrent use by several
// goroutines.
type Document struct {
	doc *pdfport.Document
}

// New opens the PDF file at filename.
func New(filename string) (*Document, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return NewFromMemory(b)
}

// NewFromMemory opens a PDF held in memory.
func NewFromMemory(b []byte) (*Document, error) {
	if len(b) == 0 {
		return nil, ErrEmptyBytes
	}
	doc, err := pdfport.Open(b)
	if err != nil {
		return nil, err
	}
	return &Document{doc: doc}, nil
}

// NewFromReader reads a PDF from r and opens it.
func NewFromReader(r io.Reader) (*Document, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return NewFromMemory(b)
}

// NumPage returns the number of pages in the document.
func (d *Document) NumPage() int {
	return d.doc.NumPages()
}

// Bound returns the crop box of the page with the given 0-based number.
func (d *Document) Bound(pageNumber int) (image.Rectangle, error) {
	return d.doc.Bound(pageNumber)
}

// Metadata returns the document information dictionary as strings.
func (d *Document) Metadata() map[string]string {
	return d.doc.Metadata()
}

// Links returns the external links on the page with the given 0-based number.
// It returns ErrUnsupported until link annotation parsing is ported.
func (d *Document) Links(pageNumber int) ([]Link, error) {
	links, err := d.doc.Links(pageNumber)
	if err != nil {
		return nil, err
	}
	out := make([]Link, len(links))
	for i, l := range links {
		out[i] = Link{URI: l.URI}
	}
	return out, nil
}

// ToC returns the table of contents. It returns ErrUnsupported until outline
// parsing is ported.
func (d *Document) ToC() ([]Outline, error) {
	toc, err := d.doc.ToC()
	if err != nil {
		return nil, err
	}
	out := make([]Outline, len(toc))
	for i, o := range toc {
		out[i] = Outline{Level: o.Level, Title: o.Title, URI: o.URI, Page: o.Page, Top: o.Top}
	}
	return out, nil
}

// Close releases the document. It is a no-op kept for API compatibility: the
// pure Go document holds no external resources.
func (d *Document) Close() error {
	return nil
}
